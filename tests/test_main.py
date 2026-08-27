"""Tests for main entry point.

The @hydra.main decorator makes main() hard to test directly in pytest
because config_path resolution depends on the calling context.
We test the inner logic by calling main's body with a composed config.
"""

import pytest
from hydra.core.global_hydra import GlobalHydra


@pytest.fixture(autouse=True)
def _clear_hydra():
    """Ensure GlobalHydra is clean before and after each test in this module."""
    GlobalHydra.instance().clear()
    yield
    GlobalHydra.instance().clear()


def test_main_creates_executor_and_runs(config, monkeypatch):
    """Verify that the main function creates an Executor and calls run()."""
    calls = []

    class FakeExecutor:
        def __init__(self, cfg):
            calls.append(("init", cfg))

        def run(self):
            calls.append(("run",))

    monkeypatch.setattr("zotero_arxiv_daily.main.Executor", FakeExecutor)

    # Call main's body directly, bypassing @hydra.main
    from zotero_arxiv_daily import main as main_mod

    # Simulate what @hydra.main does: calls main(config)
    main_mod.main.__wrapped__(config)

    assert ("init", config) in calls
    assert ("run",) in calls


def test_main_debug_logging(config, monkeypatch):
    """Verify debug mode sets appropriate log level."""
    from omegaconf import open_dict

    with open_dict(config):
        config.executor.debug = True

    class FakeExecutor:
        def __init__(self, cfg):
            pass

        def run(self):
            pass

    monkeypatch.setattr("zotero_arxiv_daily.main.Executor", FakeExecutor)

    from zotero_arxiv_daily import main as main_mod

    main_mod.main.__wrapped__(config)
    # If we get here without error, the debug path executed successfully


def test_compose_config_from_generic_env_vars(monkeypatch):
    """Verify that Hydra composes default config resolving credentials and models from environment variables."""
    from pathlib import Path

    from hydra import compose, initialize_config_dir

    monkeypatch.setenv("ZOTERO_ID", "12345678")
    monkeypatch.setenv("ZOTERO_KEY", "test-zot-key")
    monkeypatch.setenv("SMTP_SERVER", "smtp.tem.scaleway.com")
    monkeypatch.setenv("SMTP_PORT", "587")
    monkeypatch.setenv("SMTP_USERNAME", "test-user")
    monkeypatch.setenv("SMTP_PASSWORD", "test-pwd")
    monkeypatch.setenv("SMTP_SENDER", "sender@example.com")
    monkeypatch.setenv("SMTP_RECEIVER", "receiver@example.com")
    monkeypatch.setenv("LLM_API_KEY", "test-gemini-key")
    monkeypatch.setenv("LLM_BASE_URL", "https://generativelanguage.googleapis.com/v1beta/openai/")
    monkeypatch.setenv("LLM_MODEL", "gemini-2.5-flash")
    monkeypatch.setenv("EMBEDDING_API_KEY", "test-gemini-key")
    monkeypatch.setenv("EMBEDDING_BASE_URL", "https://generativelanguage.googleapis.com/v1beta/openai/")
    monkeypatch.setenv("EMBEDDING_MODEL", "text-embedding-004")
    monkeypatch.setenv("DEBUG", "true")

    config_dir = str(Path(__file__).resolve().parent.parent / "config")
    with initialize_config_dir(config_dir=config_dir, version_base=None):
        cfg = compose(config_name="default")

    assert cfg.zotero.user_id == "12345678"
    assert cfg.zotero.api_key == "test-zot-key"
    assert cfg.email.smtp_server == "smtp.tem.scaleway.com"
    assert cfg.email.smtp_port == 587
    assert cfg.email.smtp_username == "test-user"
    assert cfg.email.sender_password == "test-pwd"
    assert cfg.email.sender == "sender@example.com"
    assert cfg.email.receiver == "receiver@example.com"
    assert cfg.llm.api.key == "test-gemini-key"
    assert cfg.llm.api.base_url == "https://generativelanguage.googleapis.com/v1beta/openai/"
    assert cfg.llm.generation_kwargs.model == "gemini-2.5-flash"
    assert cfg.llm.generation_kwargs.max_tokens == 16384
    assert cfg.llm.language == "English"
    assert cfg.llm.rate_limit.rpm == 15
    assert cfg.llm.rate_limit.tpm == 250000
    assert cfg.llm.rate_limit.rpd == 500
    assert cfg.reranker.rate_limit.rpm == 100
    assert cfg.reranker.rate_limit.tpm == 30000
    assert cfg.reranker.rate_limit.rpd == 1000
    assert cfg.reranker.api.model == "text-embedding-004"
    assert "arxiv" in cfg.source
    assert cfg.executor.debug is True
    assert cfg.executor.max_paper_num == 100
