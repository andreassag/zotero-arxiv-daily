"""Tests for token counting and tokenization functionality."""


from zotero_arxiv_daily.utils import (
    add_embedding_tokens,
    add_tldr_tokens,
    count_tokens,
    count_tokens_batch,
    get_token_usage_stats,
    print_token_usage_stats,
    reset_token_usage_stats,
)

# ---------------------------------------------------------------------------
# Token counting utility tests
# ---------------------------------------------------------------------------


class TestCountTokens:
    """Test cases for the count_tokens function."""

    def test_qwen_tokenizer_basic_text(self):
        """Test Qwen tokenizer with basic English text."""
        text = "Hello world! This is a test."
        count = count_tokens(text, "qwen")
        assert count > 0
        assert isinstance(count, int)

    def test_qwen_tokenizer_empty_text(self):
        """Test Qwen tokenizer with empty text."""
        text = ""
        count = count_tokens(text, "qwen")
        assert count == 0

    def test_qwen_tokenizer_special_characters(self):
        """Test Qwen tokenizer with special characters."""
        text = "Hello @world! #test $pecial %characters & symbols"
        count = count_tokens(text, "qwen")
        assert count > 0

    def test_qwen_tokenizer_multilingual(self):
        """Test Qwen tokenizer with multilingual text."""
        text = "Hello 世界 こんにちは Mercedes-Benz"
        count = count_tokens(text, "qwen")
        assert count > 0

    def test_qwen_tokenizer_whitespace(self):
        """Test Qwen tokenizer with various whitespace patterns."""
        text = "Hello\n\nWorld\t\tTest   Spacing"
        count = count_tokens(text, "qwen")
        assert count > 0

    def test_gpt4_tokenizer(self):
        """Test GPT-4 tokenizer compatibility."""
        text = "Hello world! This is a test."
        count = count_tokens(text, "gpt4")
        assert count > 0
        assert isinstance(count, int)

    def test_cl100k_base_tokenizer(self):
        """Test cl100k_base tokenizer."""
        text = "Hello world! This is a test."
        count = count_tokens(text, "cl100k_base")
        assert count > 0
        assert isinstance(count, int)

    def test_o200k_base_tokenizer(self):
        """Test o200k_base tokenizer."""
        text = "Hello world! This is a test."
        count = count_tokens(text, "o200k_base")
        assert count > 0
        assert isinstance(count, int)

    def test_unknown_tokenizer_fallback(self):
        """Test that unknown tokenizer falls back to o200k_base."""
        text = "Hello world! This is a test."
        count = count_tokens(text, "unknown_tokenizer")
        assert count > 0
        # Should fall back to o200k_base, so count should match
        expected_count = count_tokens(text, "o200k_base")
        assert count == expected_count


class TestCountTokensBatch:
    """Test cases for the count_tokens_batch function."""

    def test_batch_counting_basic(self):
        """Test batch counting with multiple texts."""
        texts = ["Hello world", "This is a test", "Qwen tokenizer"]
        counts = count_tokens_batch(texts, "qwen")

        assert len(counts) == len(texts)
        assert all(isinstance(count, int) for count in counts)
        assert all(count > 0 for count in counts)

    def test_batch_counting_empty_list(self):
        """Test batch counting with empty list."""
        texts = []
        counts = count_tokens_batch(texts, "qwen")

        assert len(counts) == 0
        assert counts == []

    def test_batch_counting_mixed_content(self):
        """Test batch counting with mixed content."""
        texts = ["Hello", "", "World", "Test with special chars: @#$%", ""]
        counts = count_tokens_batch(texts, "qwen")

        assert len(counts) == len(texts)
        assert counts[1] == 0  # Empty string
        assert counts[4] == 0  # Empty string
        assert counts[0] > 0  # "Hello"
        assert counts[2] > 0  # "World"
        assert counts[3] > 0  # "Test with special chars: @#$%"

    def test_batch_counting_different_tokenizers(self):
        """Test batch counting with different tokenizers."""
        texts = ["Hello world", "Test text"]

        for tokenizer_type in ["qwen", "gpt4", "cl100k_base", "o200k_base"]:
            counts = count_tokens_batch(texts, tokenizer_type)
            assert len(counts) == len(texts)
            assert all(isinstance(count, int) for count in counts)


# ---------------------------------------------------------------------------
# Token usage tracking tests
# ---------------------------------------------------------------------------


class TestTokenUsageTracking:
    """Test cases for token usage tracking functionality."""

    def setup_method(self):
        """Reset token usage stats before each test."""
        reset_token_usage_stats()

    def test_initial_state(self):
        """Test that initial token usage stats are zero."""
        stats = get_token_usage_stats()

        assert stats["embedding_tokens"] == 0
        assert stats["tldr_prompt_tokens"] == 0
        assert stats["tldr_completion_tokens"] == 0
        assert stats["total_tokens"] == 0

    def test_reset_token_usage_stats(self):
        """Test that reset_token_usage_stats clears all counters."""
        # Add some token usage
        add_embedding_tokens(100)
        add_tldr_tokens(50, 25)

        # Verify we have usage
        stats = get_token_usage_stats()
        assert stats["total_tokens"] > 0

        # Reset
        reset_token_usage_stats()

        # Verify reset
        stats = get_token_usage_stats()
        assert stats["embedding_tokens"] == 0
        assert stats["tldr_prompt_tokens"] == 0
        assert stats["tldr_completion_tokens"] == 0
        assert stats["total_tokens"] == 0

    def test_add_embedding_tokens(self):
        """Test adding embedding tokens to statistics."""
        add_embedding_tokens(100)
        add_embedding_tokens(50)

        stats = get_token_usage_stats()
        assert stats["embedding_tokens"] == 150
        assert stats["total_tokens"] == 150

    def test_add_tldr_tokens(self):
        """Test adding TLDR tokens to statistics."""
        add_tldr_tokens(100, 50)  # prompt_tokens=100, completion_tokens=50

        stats = get_token_usage_stats()
        assert stats["tldr_prompt_tokens"] == 100
        assert stats["tldr_completion_tokens"] == 50
        assert stats["total_tokens"] == 150

    def test_add_mixed_tokens(self):
        """Test adding both embedding and TLDR tokens."""
        add_embedding_tokens(200)
        add_tldr_tokens(100, 50)
        add_embedding_tokens(50)
        add_tldr_tokens(75, 25)

        stats = get_token_usage_stats()
        assert stats["embedding_tokens"] == 250
        assert stats["tldr_prompt_tokens"] == 175
        assert stats["tldr_completion_tokens"] == 75
        assert stats["total_tokens"] == 500

    def test_get_token_usage_stats_returns_copy(self):
        """Test that get_token_usage_stats returns a copy, not the original."""
        add_embedding_tokens(100)

        stats = get_token_usage_stats()
        original_total = stats["total_tokens"]

        # Modify the returned dict
        stats["total_tokens"] = 999

        # Get stats again
        new_stats = get_token_usage_stats()
        assert new_stats["total_tokens"] == original_total

    def test_print_token_usage_stats(self, capsys):
        """Test that print_token_usage_stats runs without error and contains expected stats."""
        add_embedding_tokens(1000)
        add_tldr_tokens(500, 250)

        # Should not raise an exception
        print_token_usage_stats()

        # Verify the stats are correct
        stats = get_token_usage_stats()
        assert stats["embedding_tokens"] == 1000
        assert stats["tldr_prompt_tokens"] == 500
        assert stats["tldr_completion_tokens"] == 250
        assert stats["total_tokens"] == 1750


# ---------------------------------------------------------------------------
# Integration tests
# ---------------------------------------------------------------------------


class TestTokenizationIntegration:
    """Integration tests for tokenization with other modules."""

    def setup_method(self):
        """Reset token usage stats before each test."""
        reset_token_usage_stats()

    def test_protocol_integration(self, config, monkeypatch):
        """Test token counting integration with protocol module."""
        from tests.canned_responses import make_stub_openai_client
        from zotero_arxiv_daily.protocol import Paper

        # Create a test paper
        paper = Paper(
            source="arxiv",
            title="Test Paper Title",
            authors=["Author One"],
            abstract="This is a test abstract for the paper.",
            url="https://arxiv.org/abs/1234.5678",
            pdf_url="https://arxiv.org/pdf/1234.5678.pdf",
        )

        # Mock OpenAI client
        stub_client = make_stub_openai_client()
        monkeypatch.setattr("zotero_arxiv_daily.protocol.OpenAI", lambda **kw: stub_client)

        # Mock llm_params
        llm_params = {"language": "English", "generation_kwargs": {"model": "gpt-4o-mini", "max_tokens": 16384}}

        # Generate TLDR with token counting
        reset_token_usage_stats()
        paper.generate_tldr(stub_client, llm_params, tokenizer_type="qwen")

        # Check that token usage was tracked
        stats = get_token_usage_stats()
        assert stats["tldr_prompt_tokens"] > 0
        assert stats["tldr_completion_tokens"] > 0
        assert stats["total_tokens"] > 0

    def test_reranker_integration_api(self, config, monkeypatch):
        """Test token counting integration with API reranker."""
        from tests.canned_responses import make_stub_openai_client
        from zotero_arxiv_daily.reranker.api import ApiReranker

        # Mock OpenAI client
        stub_client = make_stub_openai_client()
        monkeypatch.setattr("zotero_arxiv_daily.reranker.api.OpenAI", lambda **kw: stub_client)

        # Create reranker
        reranker = ApiReranker(config)

        # Test texts
        s1 = ["First text", "Second text"]
        s2 = ["Third text", "Fourth text"]

        # Get similarity score (this will also count tokens)
        reset_token_usage_stats()
        similarity = reranker.get_similarity_score(s1, s2)

        # Check that token usage was tracked
        stats = get_token_usage_stats()
        assert stats["embedding_tokens"] > 0
        assert stats["total_tokens"] > 0
        assert similarity.shape == (2, 2)  # Should return similarity matrix
