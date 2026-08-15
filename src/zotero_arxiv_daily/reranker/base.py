import math
from abc import ABC, abstractmethod
from datetime import UTC, datetime

import numpy as np
from omegaconf import DictConfig

from ..protocol import CorpusPaper, Paper


class BaseScreener(ABC):
    def __init__(self, config: DictConfig):
        self.config = config

    def screen(self, candidates: list[Paper], corpus: list[CorpusPaper], top_k: int | None = None) -> list[Paper]:
        """Stage 1: Screening preprints via bi-encoder embedding similarity against Zotero corpus using KNN."""
        if not candidates or not corpus:
            return []

        screener_cfg = getattr(self.config, "screener", getattr(self.config, "reranker", {}))
        knn_k = getattr(screener_cfg, "knn_k", 3)
        knn_k = max(1, min(knn_k, len(corpus)))
        half_life_days = float(getattr(screener_cfg, "half_life_days", 180.0))

        # Compute recency decay weight for each corpus paper
        now = datetime.now(UTC)
        time_decay_weights = []
        for p in corpus:
            added_date = getattr(p, "added_date", None)
            if added_date is None:
                time_decay_weights.append(1.0)
            else:
                if added_date.tzinfo is None:
                    added_date = added_date.replace(tzinfo=UTC)
                age_days = max(0.0, (now - added_date).total_seconds() / 86400.0)
                w = math.pow(2.0, -age_days / max(half_life_days, 1.0))
                time_decay_weights.append(w)
        time_decay_weights = np.array(time_decay_weights, dtype=np.float32)

        cand_texts = [f"{c.title}\n{c.abstract}" for c in candidates]
        corp_texts = [f"{c.title}\n{c.abstract}" for c in corpus]

        sim = self.get_similarity_score(cand_texts, corp_texts)
        assert sim.shape == (len(candidates), len(corpus))
        sim = np.maximum(sim, 0.0)

        # KNN scoring per candidate
        weighted_sim = sim * time_decay_weights[np.newaxis, :]  # [n_candidates, n_corpus]
        for i, candidate in enumerate(candidates):
            row_weighted = weighted_sim[i]
            row_raw = sim[i]
            top_k_indices = np.argsort(row_weighted)[::-1][:knn_k]
            best_match_idx = top_k_indices[0]

            score = float(np.mean(row_weighted[top_k_indices])) * 10.0
            candidate.score = score
            candidate.screener_score = score
            candidate.matched_reference = corpus[best_match_idx]
            candidate.matched_reference_score = float(row_raw[best_match_idx])

        candidates = sorted(candidates, key=lambda x: x.score if x.score is not None else 0.0, reverse=True)
        if top_k is not None:
            return candidates[:top_k]
        return candidates

    def rerank(self, candidates: list[Paper], corpus: list[CorpusPaper]) -> list[Paper]:
        """Backwards compatibility alias for screen()."""
        return self.screen(candidates, corpus)

    @abstractmethod
    def get_similarity_score(self, s1: list[str], s2: list[str]) -> np.ndarray:
        raise NotImplementedError


# Alias for backward compatibility
BaseReranker = BaseScreener

registered_screeners = {}
registered_rerankers = registered_screeners


def register_screener(name: str):
    def decorator(cls):
        registered_screeners[name] = cls
        return cls

    return decorator


register_reranker = register_screener


def get_screener_cls(name: str) -> type[BaseScreener]:
    if name not in registered_screeners:
        raise ValueError(f"Screener {name} not found")
    return registered_screeners[name]


get_reranker_cls = get_screener_cls
