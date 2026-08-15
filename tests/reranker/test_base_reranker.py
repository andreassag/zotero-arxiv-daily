"""Tests for BaseScreener / BaseReranker: scoring, sorting, time decay, matched_reference."""

import numpy as np
import pytest

from tests.canned_responses import make_sample_corpus, make_sample_paper
from zotero_arxiv_daily.reranker.base import BaseReranker, get_reranker_cls


class StubReranker(BaseReranker):
    """Reranker with a controlled similarity matrix for deterministic tests."""

    def __init__(self, sim_matrix: np.ndarray):
        self.config = None
        self._sim = sim_matrix

    def get_similarity_score(self, s1, s2):
        return self._sim


def test_rerank_scores_and_sorts():
    corpus = make_sample_corpus(3)
    papers = [make_sample_paper(title=f"Paper {i}") for i in range(2)]

    # Paper 1 has higher similarity to all corpus papers
    sim = np.array(
        [
            [0.1, 0.1, 0.1],  # paper 0 — low
            [0.9, 0.9, 0.9],  # paper 1 — high
        ]
    )
    reranker = StubReranker(sim)
    ranked = reranker.rerank(papers, corpus)
    assert ranked[0].title == "Paper 1"
    assert ranked[1].title == "Paper 0"
    assert ranked[0].score > ranked[1].score
    assert ranked[0].matched_reference is not None


def test_rerank_time_decay_weighting():
    corpus = make_sample_corpus(3)
    # corpus[0] is 2026-01-01 (older), corpus[2] is 2026-01-03 (newer)
    papers_old = [make_sample_paper(title="P_old")]
    sim_old = np.array([[1.0, 0.0, 0.0]])  # matches oldest corpus paper
    reranker_old = StubReranker(sim_old)
    ranked_old = reranker_old.rerank(papers_old, corpus)
    score_old = ranked_old[0].score

    papers_new = [make_sample_paper(title="P_new")]
    sim_new = np.array([[0.0, 0.0, 1.0]])  # matches newest corpus paper
    reranker_new = StubReranker(sim_new)
    ranked_new = reranker_new.rerank(papers_new, corpus)
    score_new = ranked_new[0].score

    # Newest corpus paper gets higher time-decay weight, so score should be higher
    assert score_new > score_old
    assert ranked_new[0].matched_reference.title == corpus[2].title


def test_rerank_single_candidate_single_corpus():
    corpus = make_sample_corpus(1)
    papers = [make_sample_paper()]
    sim = np.array([[0.5]])
    reranker = StubReranker(sim)
    ranked = reranker.rerank(papers, corpus)
    assert len(ranked) == 1
    assert ranked[0].score is not None
    assert ranked[0].matched_reference == corpus[0]


def test_get_reranker_cls_unknown():
    with pytest.raises(ValueError, match="not found"):
        get_reranker_cls("nonexistent_reranker_xyz")
