"""Tests for zotero_arxiv_daily.executor: normalize_path_patterns, filter_corpus, fetch_zotero_corpus, E2E."""

from datetime import datetime

import pytest
from omegaconf import OmegaConf

from zotero_arxiv_daily.executor import Executor, normalize_path_patterns
from zotero_arxiv_daily.protocol import CorpusPaper

# ---------------------------------------------------------------------------
# normalize_path_patterns — migrated from test_include_path.py
# ---------------------------------------------------------------------------


def test_normalize_path_patterns_rejects_single_string_for_include_path():
    with pytest.raises(TypeError, match="config.zotero.include_path must be a list"):
        normalize_path_patterns("2026/survey/**", "include_path")


def test_normalize_path_patterns_accepts_list_config_for_include_path():
    include_path = OmegaConf.create(["2026/survey/**", "2026/reading-group/**"])
    assert normalize_path_patterns(include_path, "include_path") == [
        "2026/survey/**",
        "2026/reading-group/**",
    ]


def test_normalize_path_patterns_rejects_single_string_for_ignore_path():
    with pytest.raises(TypeError, match="config.zotero.ignore_path must be a list"):
        normalize_path_patterns("archive/**", "ignore_path")


def test_normalize_path_patterns_accepts_list_config_for_ignore_path():
    ignore_path = OmegaConf.create(["archive/**", "2025/**"])
    assert normalize_path_patterns(ignore_path, "ignore_path") == ["archive/**", "2025/**"]


def test_normalize_path_patterns_accepts_empty_list():
    assert normalize_path_patterns([], "ignore_path") == []


def test_executor_auto_detects_sources(config):
    from omegaconf import open_dict

    import zotero_arxiv_daily.retriever.arxiv_retriever
    import zotero_arxiv_daily.retriever.biorxiv_retriever  # noqa: F401

    with open_dict(config):
        config.executor.source = None
        config.source.arxiv.category = ["cs.AI"]
        config.source.biorxiv.category = ["bioinformatics"]
        config.source.medrxiv.category = None

    executor = Executor(config)
    assert set(executor.retrievers.keys()) == {"arxiv", "biorxiv"}


# ---------------------------------------------------------------------------
# filter_corpus — migrated from test_include_path.py
# ---------------------------------------------------------------------------


def _make_executor(include_patterns=None, ignore_patterns=None):
    executor = Executor.__new__(Executor)
    executor.include_path_patterns = (
        normalize_path_patterns(include_patterns, "include_path") if include_patterns else None
    )
    executor.ignore_path_patterns = normalize_path_patterns(ignore_patterns, "ignore_path") if ignore_patterns else None
    return executor


def test_filter_corpus_matches_any_path_against_any_pattern():
    executor = _make_executor(include_patterns=["2026/survey/**", "2026/reading-group/**"])
    corpus = [
        CorpusPaper(
            title="Survey Paper",
            abstract="",
            added_date=datetime(2026, 1, 1),
            paths=["2026/survey/topic-a", "archive/misc"],
        ),
        CorpusPaper(
            title="Reading Group Paper",
            abstract="",
            added_date=datetime(2026, 1, 2),
            paths=["notes/inbox", "2026/reading-group/week-1"],
        ),
        CorpusPaper(title="Excluded Paper", abstract="", added_date=datetime(2026, 1, 3), paths=["2025/other/topic"]),
    ]
    filtered = executor.filter_corpus(corpus)
    assert [p.title for p in filtered] == ["Survey Paper", "Reading Group Paper"]


def test_filter_corpus_excludes_papers_matching_ignore_path():
    executor = _make_executor(ignore_patterns=["archive/**", "2025/**"])
    corpus = [
        CorpusPaper(title="Active Paper", abstract="", added_date=datetime(2026, 1, 1), paths=["2026/survey/topic-a"]),
        CorpusPaper(title="Archived Paper", abstract="", added_date=datetime(2026, 1, 2), paths=["archive/misc"]),
        CorpusPaper(title="Old Paper", abstract="", added_date=datetime(2026, 1, 3), paths=["2025/other/topic"]),
    ]
    filtered = executor.filter_corpus(corpus)
    assert [p.title for p in filtered] == ["Active Paper"]


def test_filter_corpus_ignore_path_takes_precedence_over_include_path():
    executor = _make_executor(include_patterns=["2026/**"], ignore_patterns=["2026/ignore/**"])
    corpus = [
        CorpusPaper(
            title="Included Paper", abstract="", added_date=datetime(2026, 1, 1), paths=["2026/survey/topic-a"]
        ),
        CorpusPaper(title="Ignored Paper", abstract="", added_date=datetime(2026, 1, 2), paths=["2026/ignore/topic-b"]),
    ]
    filtered = executor.filter_corpus(corpus)
    assert [p.title for p in filtered] == ["Included Paper"]


def test_filter_corpus_no_filters_returns_all():
    executor = _make_executor()
    corpus = [
        CorpusPaper(title="Paper A", abstract="", added_date=datetime(2026, 1, 1), paths=["foo"]),
        CorpusPaper(title="Paper B", abstract="", added_date=datetime(2026, 1, 2), paths=["bar"]),
    ]
    filtered = executor.filter_corpus(corpus)
    assert filtered == corpus


# ---------------------------------------------------------------------------
# fetch_zotero_corpus
# ---------------------------------------------------------------------------


def test_fetch_zotero_corpus(config, monkeypatch):
    from tests.canned_responses import make_stub_zotero_client

    stub_zot = make_stub_zotero_client()
    monkeypatch.setattr("zotero_arxiv_daily.executor.zotero.Zotero", lambda *a, **kw: stub_zot)

    executor = Executor.__new__(Executor)
    executor.config = config
    corpus = executor.fetch_zotero_corpus()

    assert len(corpus) == 2
    assert corpus[0].title == "Stub Paper 1"
    assert "survey/topic-a" in corpus[0].paths[0]


def test_fetch_zotero_corpus_paper_with_zero_collections(config, monkeypatch):
    from tests.canned_responses import make_stub_zotero_client

    items = [
        {
            "data": {
                "title": "No Collection Paper",
                "abstractNote": "Abstract.",
                "dateAdded": "2026-03-01T00:00:00Z",
                "collections": [],
            }
        }
    ]
    stub_zot = make_stub_zotero_client(items=items)
    monkeypatch.setattr("zotero_arxiv_daily.executor.zotero.Zotero", lambda *a, **kw: stub_zot)

    executor = Executor.__new__(Executor)
    executor.config = config
    corpus = executor.fetch_zotero_corpus()

    assert len(corpus) == 1
    assert corpus[0].paths == []


# ---------------------------------------------------------------------------
# E2E: Executor.run()
# ---------------------------------------------------------------------------


def test_run_end_to_end(config, monkeypatch):
    """Full pipeline: Zotero fetch -> filter -> retrieve -> rerank -> TLDR -> email."""
    import smtplib

    from omegaconf import open_dict

    from tests.canned_responses import (
        make_sample_paper,
        make_stub_openai_client,
        make_stub_smtp,
        make_stub_zotero_client,
    )

    # Config: source=["arxiv"], reranker="api", send_empty=false
    with open_dict(config):
        config.executor.source = ["arxiv"]
        config.executor.reranker = "api"
        config.executor.send_empty = False

    # 1. Stub pyzotero
    stub_zot = make_stub_zotero_client()
    monkeypatch.setattr("zotero_arxiv_daily.executor.zotero.Zotero", lambda *a, **kw: stub_zot)

    # 2. Stub OpenAI (for reranker + TLDR/affiliations)
    stub_client = make_stub_openai_client()
    monkeypatch.setattr("zotero_arxiv_daily.executor.OpenAI", lambda **kw: stub_client)
    monkeypatch.setattr("zotero_arxiv_daily.reranker.api.OpenAI", lambda **kw: stub_client)
    retrieved = [
        make_sample_paper(title="E2E Paper 1", score=None),
        make_sample_paper(title="E2E Paper 2", score=None),
    ]

    # Import to register the arxiv retriever
    import zotero_arxiv_daily.retriever.arxiv_retriever  # noqa: F401
    from zotero_arxiv_daily.retriever.base import registered_retrievers

    monkeypatch.setattr(
        registered_retrievers["arxiv"],
        "retrieve_papers",
        lambda self: retrieved,
    )

    # 4. Stub SMTP
    sent = []
    monkeypatch.setattr(smtplib, "SMTP", make_stub_smtp(sent))

    # 5. Stub sleep (reranker/retriever)
    monkeypatch.setattr("zotero_arxiv_daily.retriever.base.sleep", lambda _: None)

    # 6. Run
    executor = Executor(config)
    executor.run()

    # Assertions
    assert len(sent) == 1, "Email should have been sent"
    _, _, email_body = sent[0]
    assert "text/html" in email_body


def test_run_no_papers_send_empty_false(config, monkeypatch):
    """When no papers are found and send_empty=false, no email is sent."""
    import smtplib

    from omegaconf import open_dict

    from tests.canned_responses import make_stub_openai_client, make_stub_smtp, make_stub_zotero_client

    with open_dict(config):
        config.executor.source = ["arxiv"]
        config.executor.reranker = "api"
        config.executor.send_empty = False

    stub_zot = make_stub_zotero_client()
    monkeypatch.setattr("zotero_arxiv_daily.executor.zotero.Zotero", lambda *a, **kw: stub_zot)

    stub_client = make_stub_openai_client()
    monkeypatch.setattr("zotero_arxiv_daily.executor.OpenAI", lambda **kw: stub_client)
    monkeypatch.setattr("zotero_arxiv_daily.reranker.api.OpenAI", lambda **kw: stub_client)

    import zotero_arxiv_daily.retriever.arxiv_retriever  # noqa: F401
    from zotero_arxiv_daily.retriever.base import registered_retrievers

    monkeypatch.setattr(registered_retrievers["arxiv"], "retrieve_papers", lambda self: [])

    sent = []
    monkeypatch.setattr(smtplib, "SMTP", make_stub_smtp(sent))
    monkeypatch.setattr("zotero_arxiv_daily.retriever.base.sleep", lambda _: None)

    executor = Executor(config)
    executor.run()

    assert len(sent) == 0, "No email should be sent when no papers and send_empty=false"


def test_run_no_papers_send_empty_true(config, monkeypatch):
    """When no papers are found and send_empty=true, empty email is sent."""
    import smtplib

    from omegaconf import open_dict

    from tests.canned_responses import make_stub_openai_client, make_stub_smtp, make_stub_zotero_client

    with open_dict(config):
        config.executor.source = ["arxiv"]
        config.executor.reranker = "api"
        config.executor.send_empty = True

    stub_zot = make_stub_zotero_client()
    monkeypatch.setattr("zotero_arxiv_daily.executor.zotero.Zotero", lambda *a, **kw: stub_zot)

    stub_client = make_stub_openai_client()
    monkeypatch.setattr("zotero_arxiv_daily.executor.OpenAI", lambda **kw: stub_client)
    monkeypatch.setattr("zotero_arxiv_daily.reranker.api.OpenAI", lambda **kw: stub_client)

    import zotero_arxiv_daily.retriever.arxiv_retriever  # noqa: F401
    from zotero_arxiv_daily.retriever.base import registered_retrievers

    monkeypatch.setattr(registered_retrievers["arxiv"], "retrieve_papers", lambda self: [])

    sent = []
    monkeypatch.setattr(smtplib, "SMTP", make_stub_smtp(sent))
    monkeypatch.setattr("zotero_arxiv_daily.retriever.base.sleep", lambda _: None)

    executor = Executor(config)
    executor.run()

    assert len(sent) == 1, "Email should be sent even with no papers when send_empty=true"
    _, _, body = sent[0]
    assert "text/html" in body


# ---------------------------------------------------------------------------
# No-email functionality tests
# ---------------------------------------------------------------------------


def test_run_no_email_environment_variable(config, monkeypatch):
    """When NO_EMAIL environment variable is set, email is not sent but content is generated and logged."""
    import smtplib

    from omegaconf import open_dict

    from tests.canned_responses import (
        make_sample_paper,
        make_stub_openai_client,
        make_stub_smtp,
        make_stub_zotero_client,
    )

    with open_dict(config):
        config.executor.source = ["arxiv"]
        config.executor.reranker = "api"
        config.executor.send_empty = False

    # Set NO_EMAIL environment variable
    monkeypatch.setenv("NO_EMAIL", "true")

    stub_zot = make_stub_zotero_client()
    monkeypatch.setattr("zotero_arxiv_daily.executor.zotero.Zotero", lambda *a, **kw: stub_zot)

    stub_client = make_stub_openai_client()
    monkeypatch.setattr("zotero_arxiv_daily.executor.OpenAI", lambda **kw: stub_client)
    monkeypatch.setattr("zotero_arxiv_daily.reranker.api.OpenAI", lambda **kw: stub_client)

    # Import to register the arxiv retriever
    import zotero_arxiv_daily.retriever.arxiv_retriever  # noqa: F401
    from zotero_arxiv_daily.retriever.base import registered_retrievers

    retrieved = [make_sample_paper(title="Test Paper", score=None)]
    monkeypatch.setattr(registered_retrievers["arxiv"], "retrieve_papers", lambda self: retrieved)

    sent = []
    monkeypatch.setattr(smtplib, "SMTP", make_stub_smtp(sent))
    monkeypatch.setattr("zotero_arxiv_daily.retriever.base.sleep", lambda _: None)

    # Run executor
    executor = Executor(config)
    executor.run()

    # No email should be sent
    assert len(sent) == 0, "No email should be sent when NO_EMAIL=true"


def test_run_no_email_false(config, monkeypatch):
    """When NO_EMAIL environment variable is not set or false, email is sent normally."""
    import os
    import smtplib

    from omegaconf import open_dict

    from tests.canned_responses import (
        make_sample_paper,
        make_stub_openai_client,
        make_stub_smtp,
        make_stub_zotero_client,
    )

    with open_dict(config):
        config.executor.source = ["arxiv"]
        config.executor.reranker = "api"
        config.executor.send_empty = False

    # Make sure NO_EMAIL is not set
    if "NO_EMAIL" in os.environ:
        monkeypatch.delenv("NO_EMAIL")

    stub_zot = make_stub_zotero_client()
    monkeypatch.setattr("zotero_arxiv_daily.executor.zotero.Zotero", lambda *a, **kw: stub_zot)

    stub_client = make_stub_openai_client()
    monkeypatch.setattr("zotero_arxiv_daily.executor.OpenAI", lambda **kw: stub_client)
    monkeypatch.setattr("zotero_arxiv_daily.reranker.api.OpenAI", lambda **kw: stub_client)

    # Import to register the arxiv retriever
    import zotero_arxiv_daily.retriever.arxiv_retriever  # noqa: F401
    from zotero_arxiv_daily.retriever.base import registered_retrievers

    retrieved = [make_sample_paper(title="Test Paper", score=None)]
    monkeypatch.setattr(registered_retrievers["arxiv"], "retrieve_papers", lambda self: retrieved)

    sent = []
    monkeypatch.setattr(smtplib, "SMTP", make_stub_smtp(sent))
    monkeypatch.setattr("zotero_arxiv_daily.retriever.base.sleep", lambda _: None)

    # Run executor
    executor = Executor(config)
    executor.run()

    # Email should be sent
    assert len(sent) == 1, "Email should be sent when NO_EMAIL is not set"
    _, _, body = sent[0]
    assert "text/html" in body


def test_run_no_email_with_save_path(config, monkeypatch, tmp_path):
    """When NO_EMAIL=true and SAVE_EMAIL_PATH is set, email content is saved to file."""
    import os
    import smtplib

    from omegaconf import open_dict

    from tests.canned_responses import (
        make_sample_paper,
        make_stub_openai_client,
        make_stub_smtp,
        make_stub_zotero_client,
    )

    with open_dict(config):
        config.executor.source = ["arxiv"]
        config.executor.reranker = "api"
        config.executor.send_empty = False

    # Set NO_EMAIL and SAVE_EMAIL_PATH environment variables
    monkeypatch.setenv("NO_EMAIL", "true")
    save_path = str(tmp_path / "email_output.html")
    monkeypatch.setenv("SAVE_EMAIL_PATH", save_path)

    stub_zot = make_stub_zotero_client()
    monkeypatch.setattr("zotero_arxiv_daily.executor.zotero.Zotero", lambda *a, **kw: stub_zot)

    stub_client = make_stub_openai_client()
    monkeypatch.setattr("zotero_arxiv_daily.executor.OpenAI", lambda **kw: stub_client)
    monkeypatch.setattr("zotero_arxiv_daily.reranker.api.OpenAI", lambda **kw: stub_client)

    # Import to register the arxiv retriever
    import zotero_arxiv_daily.retriever.arxiv_retriever  # noqa: F401
    from zotero_arxiv_daily.retriever.base import registered_retrievers

    retrieved = [make_sample_paper(title="Test Paper", score=None)]
    monkeypatch.setattr(registered_retrievers["arxiv"], "retrieve_papers", lambda self: retrieved)

    sent = []
    monkeypatch.setattr(smtplib, "SMTP", make_stub_smtp(sent))
    monkeypatch.setattr("zotero_arxiv_daily.retriever.base.sleep", lambda _: None)

    # Run executor
    executor = Executor(config)
    executor.run()

    # No email should be sent
    assert len(sent) == 0, "No email should be sent when NO_EMAIL=true"

    # File should be created and contain HTML content
    assert os.path.exists(save_path), "Email file should be saved"
    with open(save_path, encoding="utf-8") as f:
        content = f.read()
    assert "<html>" in content or "html>" in content, "Saved file should contain HTML content"


def test_run_no_email_variations(config, monkeypatch):
    """Test various NO_EMAIL environment variable values."""
    import smtplib

    from omegaconf import open_dict

    from tests.canned_responses import (
        make_sample_paper,
        make_stub_openai_client,
        make_stub_smtp,
        make_stub_zotero_client,
    )

    # Test different NO_EMAIL values that should prevent email sending
    no_email_values = ["true", "1", "yes", "True", "TRUE"]

    for no_email_value in no_email_values:
        with open_dict(config):
            config.executor.source = ["arxiv"]
            config.executor.reranker = "api"
            config.executor.send_empty = False

        # Set NO_EMAIL environment variable
        monkeypatch.setenv("NO_EMAIL", no_email_value)

        stub_zot = make_stub_zotero_client()
        monkeypatch.setattr("zotero_arxiv_daily.executor.zotero.Zotero", lambda *a, _zot=stub_zot, **kw: _zot)

        stub_client = make_stub_openai_client()
        monkeypatch.setattr("zotero_arxiv_daily.executor.OpenAI", lambda _client=stub_client, **kw: _client)
        monkeypatch.setattr("zotero_arxiv_daily.reranker.api.OpenAI", lambda _client=stub_client, **kw: _client)

        # Import to register the arxiv retriever
        import zotero_arxiv_daily.retriever.arxiv_retriever  # noqa: F401
        from zotero_arxiv_daily.retriever.base import registered_retrievers

        retrieved = [make_sample_paper(title="Test Paper", score=None)]
        monkeypatch.setattr(registered_retrievers["arxiv"], "retrieve_papers", lambda self, _ret=retrieved: _ret)

        sent = []
        monkeypatch.setattr(smtplib, "SMTP", make_stub_smtp(sent))
        monkeypatch.setattr("zotero_arxiv_daily.retriever.base.sleep", lambda _: None)

        # Run executor
        executor = Executor(config)
        executor.run()

        # No email should be sent for any of these values
        assert len(sent) == 0, f"No email should be sent when NO_EMAIL={no_email_value}"

    # Test values that should NOT prevent email sending
    normal_values = ["false", "0", "no", "", "random"]

    for normal_value in normal_values:
        # Clean up from previous test
        sent = []
        monkeypatch.setattr(smtplib, "SMTP", make_stub_smtp(sent))

        with open_dict(config):
            config.executor.source = ["arxiv"]
            config.executor.reranker = "api"
            config.executor.send_empty = False

        # Set NO_EMAIL environment variable to a value that should NOT prevent sending
        monkeypatch.setenv("NO_EMAIL", normal_value)

        stub_zot = make_stub_zotero_client()
        monkeypatch.setattr("zotero_arxiv_daily.executor.zotero.Zotero", lambda *a, _zot=stub_zot, **kw: _zot)

        stub_client = make_stub_openai_client()
        monkeypatch.setattr("zotero_arxiv_daily.executor.OpenAI", lambda _client=stub_client, **kw: _client)
        monkeypatch.setattr("zotero_arxiv_daily.reranker.api.OpenAI", lambda _client=stub_client, **kw: _client)

        from zotero_arxiv_daily.retriever.base import registered_retrievers

        retrieved = [make_sample_paper(title="Test Paper", score=None)]
        monkeypatch.setattr(registered_retrievers["arxiv"], "retrieve_papers", lambda self, _ret=retrieved: _ret)

        monkeypatch.setattr("zotero_arxiv_daily.retriever.base.sleep", lambda _: None)

        # Run executor
        executor = Executor(config)
        executor.run()

        # Email should be sent for these values
        assert len(sent) == 1, f"Email should be sent when NO_EMAIL={normal_value}"

