import math
from abc import ABC, abstractmethod
from collections import defaultdict

import requests
from loguru import logger
from omegaconf import DictConfig

from ..protocol import CorpusPaper, Paper


def calibrate_cross_encoder_score(raw_score: float) -> float:
    """Normalize raw cross-encoder score to [0.0, 10.0] scale."""
    if 0.0 <= raw_score <= 1.0:
        return float(raw_score * 10.0)
    try:
        # Sigmoid calibration for logit scores
        prob = 1.0 / (1.0 + math.exp(-raw_score))
        return float(prob * 10.0)
    except OverflowError:
        return 10.0 if raw_score > 0 else 0.0


class BaseCrossEncoderReranker(ABC):
    def __init__(self, config: DictConfig):
        self.config = config
        self.reranker_cfg = getattr(config, "reranker", {})

    def rerank(self, candidates: list[Paper], corpus: list[CorpusPaper]) -> list[Paper]:
        """Stage 2: Target-aware pairwise cross-encoder reranking against matched Zotero reference papers."""
        if not candidates:
            return []

        top_k = getattr(self.config.executor, "max_paper_num", getattr(self.reranker_cfg, "top_k", 10))
        blend_weight = float(getattr(self.reranker_cfg, "blend_weight", 0.7))
        explicit_interest = getattr(self.reranker_cfg, "research_interest", None)

        logger.info(f"Stage 2 Reranking {len(candidates)} candidates using target-aware cross-encoder...")

        # Fallback reference if candidate has no matched_reference
        fallback_query = ""
        if explicit_interest:
            fallback_query = str(explicit_interest)
        elif corpus:
            recent_papers = sorted(corpus, key=lambda x: x.added_date, reverse=True)
            fallback_query = f"{recent_papers[0].title}\n{recent_papers[0].abstract}".strip()

        # Group candidates by query text for efficient batched API calls
        query_to_items: dict[str, list[tuple[int, str]]] = defaultdict(list)
        for idx, cand in enumerate(candidates):
            if explicit_interest:
                q_text = str(explicit_interest)
            elif cand.matched_reference is not None:
                q_text = f"{cand.matched_reference.title}\n{cand.matched_reference.abstract}".strip()
            else:
                q_text = fallback_query

            doc_text = f"{cand.title}\n{cand.abstract}".strip()
            query_to_items[q_text].append((idx, doc_text))

        raw_scores = [0.0] * len(candidates)
        for query_text, items in query_to_items.items():
            if not query_text:
                continue
            cand_indices = [item[0] for item in items]
            docs = [item[1] for item in items]
            batch_scores = self.score_pairs(query_text, docs)
            for cand_idx, score in zip(cand_indices, batch_scores, strict=True):
                raw_scores[cand_idx] = score

        # Score blending & calibration
        for cand, raw_score in zip(candidates, raw_scores, strict=True):
            ce_score = calibrate_cross_encoder_score(raw_score)
            screener_score = cand.screener_score if cand.screener_score is not None else (cand.score or 0.0)
            final_score = blend_weight * ce_score + (1.0 - blend_weight) * screener_score
            cand.score = round(float(final_score), 2)

        candidates.sort(key=lambda x: x.score if x.score is not None else 0.0, reverse=True)
        return candidates[:top_k]

    @abstractmethod
    def score_pairs(self, query: str, docs: list[str]) -> list[float]:
        raise NotImplementedError


class CrossEncoderReranker(BaseCrossEncoderReranker):
    """Reranker using SiliconFlow or standard Cohere/Jina/OpenAI-compatible /v1/rerank REST API endpoint."""

    def score_pairs(self, query: str, docs: list[str]) -> list[float]:
        api_cfg = getattr(self.reranker_cfg, "api", self.reranker_cfg)
        base_url = getattr(api_cfg, "base_url", getattr(self.config.llm.api, "base_url", "")).rstrip("/")
        api_key = getattr(api_cfg, "key", getattr(self.config.llm.api, "key", ""))
        model_name = getattr(api_cfg, "model", "Qwen/Qwen3-Reranker-8B")

        if not base_url.endswith("/rerank"):
            if base_url.endswith("/v1"):
                url = f"{base_url}/rerank"
            else:
                url = f"{base_url}/v1/rerank"
        else:
            url = base_url

        headers = {
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json",
        }
        payload = {
            "model": model_name,
            "query": query,
            "documents": docs,
            "top_n": len(docs),
        }

        try:
            response = requests.post(url, json=payload, headers=headers, timeout=60)
            response.raise_for_status()
            data = response.json()

            results = data.get("results", data.get("data", []))
            scores = [0.0] * len(docs)
            for item in results:
                idx = item.get("index", 0)
                score = item.get("relevance_score", item.get("score", 0.0))
                if 0 <= idx < len(docs):
                    scores[idx] = float(score)

            return scores
        except Exception as exc:
            logger.error(f"Rerank API call failed: {exc}. Falling back to default scores.")
            return [0.0 for _ in docs]


# Compatibility alias
ApiCrossEncoderReranker = CrossEncoderReranker


def get_cross_encoder_reranker(config: DictConfig) -> CrossEncoderReranker | None:
    reranker_cfg = getattr(config, "reranker", None)
    if not reranker_cfg or not getattr(reranker_cfg, "enabled", True):
        return None

    return CrossEncoderReranker(config)
