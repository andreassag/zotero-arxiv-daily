import threading
import time
from typing import Any

from loguru import logger


class RateLimiter:
    """Sliding-window rate limiter enforcing RPM, TPM (input tokens), and RPD."""

    def __init__(
        self,
        rpm: int | None = None,
        tpm: int | None = None,
        rpd: int | None = None,
        name: str = "LLM",
    ):
        self.rpm = rpm
        self.tpm = tpm
        self.rpd = rpd
        self.name = name

        self._lock = threading.Lock()
        self._minute_requests: list[float] = []
        self._minute_tokens: list[tuple[float, int]] = []
        self._day_requests: list[float] = []

    @staticmethod
    def estimate_tokens(data: Any) -> int:
        """Lightweight token estimator (~4 chars per token)."""
        if isinstance(data, str):
            return max(1, len(data) // 4)
        elif isinstance(data, list):
            return sum(RateLimiter.estimate_tokens(item) for item in data)
        elif isinstance(data, dict):
            return sum(RateLimiter.estimate_tokens(v) for v in data.values())
        return 1

    def acquire(self, estimated_tokens: int = 1) -> None:
        """Wait until rate limits permit making a request, then record it."""
        with self._lock:
            # 1. Enforce Requests Per Day (RPD)
            now = time.time()
            self._day_requests = [t for t in self._day_requests if now - t < 86400.0]
            if self.rpd and len(self._day_requests) >= self.rpd:
                raise RuntimeError(f"[{self.name}] Daily request limit ({self.rpd} RPD) exceeded.")

            # 2. Enforce Requests Per Minute (RPM) & Tokens Per Minute (TPM)
            while True:
                now = time.time()
                self._minute_requests = [t for t in self._minute_requests if now - t < 60.0]
                self._minute_tokens = [(t, count) for (t, count) in self._minute_tokens if now - t < 60.0]

                rpm_wait = 0.0
                tpm_wait = 0.0

                if self.rpm and len(self._minute_requests) >= self.rpm:
                    oldest_req = self._minute_requests[0]
                    rpm_wait = max(0.1, 60.0 - (now - oldest_req) + 1.0)

                current_tokens = sum(c for _, c in self._minute_tokens)
                if self.tpm and (current_tokens + estimated_tokens) > self.tpm:
                    if self._minute_tokens:
                        oldest_token_time = self._minute_tokens[0][0]
                        tpm_wait = max(0.1, 60.0 - (now - oldest_token_time) + 1.0)

                wait_time = max(rpm_wait, tpm_wait)
                if wait_time > 0:
                    logger.info(
                        f"[{self.name}] Rate limit approaching (RPM: {len(self._minute_requests)}/{self.rpm or 'unlimited'}, "
                        f"TPM: {current_tokens + estimated_tokens}/{self.tpm or 'unlimited'}). Waiting {wait_time:.1f}s..."
                    )
                    time.sleep(wait_time)
                else:
                    break

            now = time.time()
            self._minute_requests.append(now)
            self._minute_tokens.append((now, estimated_tokens))
            self._day_requests.append(now)
