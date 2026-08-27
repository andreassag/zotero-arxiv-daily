import time

import numpy as np
from loguru import logger
from openai import OpenAI, RateLimitError

from ..rate_limiter import RateLimiter
from .base import BaseReranker, register_reranker


@register_reranker("api")
class ApiReranker(BaseReranker):
    def __init__(self, config):
        super().__init__(config)
        rate_limit_cfg = self.config.get("reranker", {}).get("rate_limit", {})
        self.rate_limiter = RateLimiter(
            rpm=rate_limit_cfg.get("rpm", 100) if rate_limit_cfg else None,
            tpm=rate_limit_cfg.get("tpm", 30000) if rate_limit_cfg else None,
            rpd=rate_limit_cfg.get("rpd", 1000) if rate_limit_cfg else None,
            name="Embedding",
        )

    def get_similarity_score(self, s1: list[str], s2: list[str]) -> np.ndarray:
        client = OpenAI(
            api_key=self.config.reranker.api.key,
            base_url=self.config.reranker.api.base_url,
        )
        batch_size = self.config.reranker.api.get("batch_size") or 32
        all_texts = s1 + s2
        all_embeddings = []
        for i in range(0, len(all_texts), batch_size):
            batch = all_texts[i : i + batch_size]
            est_tokens = self.rate_limiter.estimate_tokens(batch)
            self.rate_limiter.acquire(est_tokens)

            max_retries = 5
            for attempt in range(max_retries):
                try:
                    response = client.embeddings.create(
                        input=batch,
                        model=self.config.reranker.api.model,
                    )
                    all_embeddings.extend([r.embedding for r in response.data])
                    break
                except (RateLimitError, Exception) as e:
                    if "429" in str(e) or isinstance(e, RateLimitError):
                        wait_sec = 55.0
                        if attempt < max_retries - 1:
                            logger.warning(
                                f"[Embedding] Rate limited by API (429). Retrying in {wait_sec:.0f}s (attempt {attempt + 1}/{max_retries})..."
                            )
                            time.sleep(wait_sec)
                            continue
                    raise e

        s1_embeddings = np.array(all_embeddings[: len(s1)])  # [n_s1, d]
        s2_embeddings = np.array(all_embeddings[len(s1) :])  # [n_s2, d]
        s1_embeddings_normalized = s1_embeddings / np.linalg.norm(s1_embeddings, axis=1, keepdims=True)
        s2_embeddings_normalized = s2_embeddings / np.linalg.norm(s2_embeddings, axis=1, keepdims=True)
        sim = np.dot(s1_embeddings_normalized, s2_embeddings_normalized.T)  # [n_s1, n_s2]
        return sim
