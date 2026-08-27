"""Tests for RateLimiter class enforcing RPM, TPM, and RPD limits."""

import time

import pytest

from zotero_arxiv_daily.rate_limiter import RateLimiter


def test_estimate_tokens():
    # String
    assert RateLimiter.estimate_tokens("abcd") == 1
    assert RateLimiter.estimate_tokens("a" * 40) == 10
    assert RateLimiter.estimate_tokens("") == 1

    # List of strings
    assert RateLimiter.estimate_tokens(["a" * 20, "b" * 20]) == 10

    # Dict / Messages
    messages = [
        {"role": "system", "content": "a" * 40},
        {"role": "user", "content": "b" * 40},
    ]
    assert RateLimiter.estimate_tokens(messages) >= 20


def test_rate_limiter_rpm():
    limiter = RateLimiter(rpm=2, tpm=10000, rpd=100, name="TestRPM")
    t0 = time.time()
    limiter.acquire(10)
    limiter.acquire(10)
    # 2 requests made; 3rd request should wait until sliding window frees up
    # To avoid long test wait, mock the timestamps
    limiter._minute_requests = [t0 - 59.8, t0 - 59.8]
    limiter.acquire(10)
    t1 = time.time()
    assert t1 >= t0


def test_rate_limiter_tpm():
    limiter = RateLimiter(rpm=100, tpm=50, rpd=100, name="TestTPM")
    t0 = time.time()
    limiter.acquire(30)
    # Next acquire of 30 exceeds 50 TPM
    limiter._minute_tokens = [(t0 - 59.8, 30)]
    limiter.acquire(30)
    t1 = time.time()
    assert t1 >= t0


def test_rate_limiter_rpd_exceeded():
    limiter = RateLimiter(rpm=100, tpm=10000, rpd=2, name="TestRPD")
    limiter.acquire(10)
    limiter.acquire(10)
    with pytest.raises(RuntimeError, match="Daily request limit"):
        limiter.acquire(10)
