import json
import os

import numpy as np
from loguru import logger
from openai import OpenAI

from ..utils import add_embedding_tokens, count_tokens_batch
from .base import BaseReranker, register_reranker


@register_reranker("api")
class ApiReranker(BaseReranker):
    def get_similarity_score(self, s1: list[str], s2: list[str]) -> np.ndarray:
        # Save the input data for debugging/inspection
        self._save_embedding_input_data(s1, s2)

        client = OpenAI(api_key=self.config.reranker.api.key, base_url=self.config.reranker.api.base_url)
        batch_size = self.config.reranker.api.get("batch_size") or 64
        all_texts = s1 + s2
        all_embeddings = []

        # Count tokens for embedding input using Qwen tokenizer
        tokenizer_type = self.config.reranker.api.get("tokenizer_type", "qwen")

        for i in range(0, len(all_texts), batch_size):
            batch = all_texts[i : i + batch_size]

            # Count tokens for this batch and add to statistics
            batch_token_counts = count_tokens_batch(batch, tokenizer_type)
            total_batch_tokens = sum(batch_token_counts)
            add_embedding_tokens(total_batch_tokens)
            logger.debug(f"Embedding batch {i // batch_size + 1} token count: {total_batch_tokens}")

            response = client.embeddings.create(input=batch, model=self.config.reranker.api.model)
            all_embeddings.extend([r.embedding for r in response.data])

        s1_embeddings = np.array(all_embeddings[: len(s1)])  # [n_s1, d]
        s2_embeddings = np.array(all_embeddings[len(s1) :])  # [n_s2, d]
        s1_embeddings_normalized = s1_embeddings / np.linalg.norm(s1_embeddings, axis=1, keepdims=True)
        s2_embeddings_normalized = s2_embeddings / np.linalg.norm(s2_embeddings, axis=1, keepdims=True)
        sim = np.dot(s1_embeddings_normalized, s2_embeddings_normalized.T)  # [n_s1, n_s2]
        return sim

    def _save_embedding_input_data(self, s1: list[str], s2: list[str]):
        """Save the input data (s1 and s2) to files for debugging and inspection."""
        try:
            # Create outputs directory if it doesn't exist
            os.makedirs("outputs", exist_ok=True)

            # Save s1 (candidate abstracts)
            s1_path = "outputs/embedding_input_s1.json"
            with open(s1_path, "w", encoding="utf-8") as f:
                json.dump(s1, f, ensure_ascii=False, indent=2)
            logger.info(f"Saved s1 data to: {s1_path}")

            # Save s2 (corpus abstracts)
            s2_path = "outputs/embedding_input_s2.json"
            with open(s2_path, "w", encoding="utf-8") as f:
                json.dump(s2, f, ensure_ascii=False, indent=2)
            logger.info(f"Saved s2 data to: {s2_path}")

        except Exception as e:
            logger.error(f"Failed to save embedding input data: {e}")
