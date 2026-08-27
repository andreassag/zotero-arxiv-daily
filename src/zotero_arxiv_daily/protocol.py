import json
import re
import time
from dataclasses import dataclass
from datetime import datetime
from typing import Optional, TypeVar

from loguru import logger
from openai import OpenAI, RateLimitError

from .rate_limiter import RateLimiter

RawPaperItem = TypeVar("RawPaperItem")


@dataclass
class Paper:
    source: str
    title: str
    authors: list[str]
    abstract: str
    url: str
    pdf_url: Optional[str] = None
    full_text: Optional[str] = None
    tldr: Optional[str] = None
    affiliations: Optional[list[str]] = None
    score: Optional[float] = None

    def _generate_tldr_with_llm(
        self,
        llm_client: OpenAI,
        llm_params: dict,
        rate_limiter: Optional[RateLimiter] = None,
    ) -> str:
        lang = llm_params.get("language", "English")
        prompt = f"Given the following information of a paper, generate a one-sentence TLDR summary in {lang}:\n\n"
        if self.title:
            prompt += f"Title:\n {self.title}\n\n"

        if self.abstract:
            prompt += f"Abstract: {self.abstract}\n\n"

        if self.full_text:
            prompt += f"Preview of main content:\n {self.full_text}\n\n"

        if not self.full_text and not self.abstract:
            logger.warning(f"Neither full text nor abstract is provided for {self.url}")
            return "Failed to generate TLDR. Neither full text nor abstract is provided"

        messages = [
            {
                "role": "system",
                "content": f"You are an assistant who perfectly summarizes scientific paper, and gives the core idea of the paper to the user. Your answer should be in {lang}.",
            },
            {"role": "user", "content": prompt},
        ]

        if rate_limiter:
            est_tokens = RateLimiter.estimate_tokens(messages)
            rate_limiter.acquire(est_tokens)

        max_retries = 5
        for attempt in range(max_retries):
            try:
                response = llm_client.chat.completions.create(
                    messages=messages,
                    **llm_params.get("generation_kwargs", {}),
                )
                tldr = response.choices[0].message.content
                return tldr
            except (RateLimitError, Exception) as e:
                if ("429" in str(e) or isinstance(e, RateLimitError)) and attempt < max_retries - 1:
                    wait_sec = 55.0
                    logger.warning(
                        f"[LLM-Gen] Rate limited by API (429). Retrying in {wait_sec:.0f}s (attempt {attempt + 1}/{max_retries})..."
                    )
                    time.sleep(wait_sec)
                    continue
                raise e

    def generate_tldr(
        self,
        llm_client: OpenAI,
        llm_params: dict,
        rate_limiter: Optional[RateLimiter] = None,
    ) -> str:
        try:
            tldr = self._generate_tldr_with_llm(llm_client, llm_params, rate_limiter=rate_limiter)
            self.tldr = tldr
            return tldr
        except Exception as e:
            logger.warning(f"Failed to generate tldr of {self.url}: {e}")
            tldr = self.abstract
            self.tldr = tldr
            return tldr

    def _generate_affiliations_with_llm(
        self,
        llm_client: OpenAI,
        llm_params: dict,
        rate_limiter: Optional[RateLimiter] = None,
    ) -> Optional[list[str]]:
        if self.full_text is not None:
            prompt = (
                f"Given the beginning of a paper, extract the affiliations of the authors in a python list format, "
                f"which is sorted by the author order. If there is no affiliation found, return an empty list '[]':\n\n{self.full_text}"
            )
            messages = [
                {
                    "role": "system",
                    "content": (
                        "You are an assistant who perfectly extracts affiliations of authors from a paper. "
                        'You should return a python list of affiliations sorted by the author order, like ["TsingHua University","Peking University"]. '
                        "If an affiliation is consisted of multi-level affiliations, like 'Department of Computer Science, TsingHua University', "
                        "you should return the top-level affiliation 'TsingHua University' only. Do not contain duplicated affiliations. "
                        "If there is no affiliation found, you should return an empty list [ ]. "
                        "You should only return the final list of affiliations, and do not return any intermediate results."
                    ),
                },
                {"role": "user", "content": prompt},
            ]

            if rate_limiter:
                est_tokens = RateLimiter.estimate_tokens(messages)
                rate_limiter.acquire(est_tokens)

            raw_content = ""
            max_retries = 5
            for attempt in range(max_retries):
                try:
                    response = llm_client.chat.completions.create(
                        messages=messages,
                        **llm_params.get("generation_kwargs", {}),
                    )
                    raw_content = response.choices[0].message.content or ""
                    break
                except (RateLimitError, Exception) as e:
                    if ("429" in str(e) or isinstance(e, RateLimitError)) and attempt < max_retries - 1:
                        wait_sec = 55.0
                        logger.warning(
                            f"[LLM-Gen] Rate limited by API (429). Retrying in {wait_sec:.0f}s (attempt {attempt + 1}/{max_retries})..."
                        )
                        time.sleep(wait_sec)
                        continue
                    raise e

            match = re.search(r"\[.*?\]", raw_content, flags=re.DOTALL)
            if not match:
                return None

            list_str = match.group(0)
            parsed = None
            try:
                parsed = json.loads(list_str)
            except Exception:
                try:
                    import ast

                    parsed = ast.literal_eval(list_str)
                except Exception:
                    pass

            if isinstance(parsed, list):
                seen = set()
                result = []
                for a in parsed:
                    if a and str(a) not in seen:
                        seen.add(str(a))
                        result.append(str(a))
                return result or None
            return None

    def generate_affiliations(
        self,
        llm_client: OpenAI,
        llm_params: dict,
        rate_limiter: Optional[RateLimiter] = None,
    ) -> Optional[list[str]]:
        try:
            affiliations = self._generate_affiliations_with_llm(llm_client, llm_params, rate_limiter=rate_limiter)
            self.affiliations = affiliations
            return affiliations
        except Exception as e:
            logger.debug(f"Failed to generate affiliations of {self.url}: {e}")
            self.affiliations = None
            return None


@dataclass
class CorpusPaper:
    title: str
    abstract: str
    added_date: datetime
    paths: list[str]
