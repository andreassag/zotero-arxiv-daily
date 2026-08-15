import requests

from tests.canned_responses import make_sample_corpus, make_sample_paper
from zotero_arxiv_daily.reranker.cross_encoder import (
    calibrate_cross_encoder_score,
    get_cross_encoder_reranker,
)


def test_calibrate_cross_encoder_score():
    # In [0, 1] range: scaled to [0, 10]
    assert calibrate_cross_encoder_score(0.85) == 8.5
    assert calibrate_cross_encoder_score(0.0) == 0.0
    assert calibrate_cross_encoder_score(1.0) == 10.0

    # Logit scale (negative or > 1): sigmoid mapping
    assert 0.0 <= calibrate_cross_encoder_score(-5.0) <= 1.0
    assert 9.0 <= calibrate_cross_encoder_score(5.0) <= 10.0


def test_api_cross_encoder_reranker_with_matched_reference(config, monkeypatch):
    config.reranker.enabled = True
    config.executor.max_paper_num = 2
    config.reranker.model = "Qwen/Qwen3-Reranker-8B"
    config.reranker.blend_weight = 0.7

    captured_requests = []

    class StubResponse:
        def raise_for_status(self):
            pass

        def json(self):
            return {
                "results": [
                    {"index": 0, "relevance_score": 0.3},
                    {"index": 1, "relevance_score": 0.9},
                    {"index": 2, "relevance_score": 0.5},
                ]
            }

    def stub_post(url, json, headers, timeout):
        captured_requests.append(json)
        assert json["model"] == "Qwen/Qwen3-Reranker-8B"
        return StubResponse()

    monkeypatch.setattr(requests, "post", stub_post)

    reranker = get_cross_encoder_reranker(config)
    assert reranker is not None

    corpus = make_sample_corpus(3)
    c1 = make_sample_paper(title="Low Relevance", score=3.0)
    c1.matched_reference = corpus[0]
    c2 = make_sample_paper(title="High Relevance", score=8.0)
    c2.matched_reference = corpus[0]
    c3 = make_sample_paper(title="Mid Relevance", score=5.0)
    c3.matched_reference = corpus[0]

    candidates = [c1, c2, c3]

    reranked = reranker.rerank(candidates, corpus)
    assert len(reranked) == 2  # Truncated to top_k = 2
    assert reranked[0].title == "High Relevance"
    assert reranked[1].title == "Mid Relevance"
    assert len(captured_requests) == 1
    assert corpus[0].title in captured_requests[0]["query"]


def test_cross_encoder_disabled_when_config_false(config):
    config.reranker.enabled = False
    reranker = get_cross_encoder_reranker(config)
    assert reranker is None
