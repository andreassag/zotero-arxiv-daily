from __future__ import annotations

import json
import re
from dataclasses import dataclass
from datetime import datetime
from typing import TypeVar

import tiktoken
from loguru import logger
from openai import OpenAI

from .utils import add_tldr_tokens, count_tokens

RawPaperItem = TypeVar("RawPaperItem")

_ENC_O200K = tiktoken.get_encoding("o200k_base")
_ENC_GPT4O = tiktoken.encoding_for_model("gpt-4o")


@dataclass
class Paper:
    source: str
    title: str
    authors: list[str]
    abstract: str
    url: str
    pdf_url: str | None = None
    full_text: str | None = None
    tldr: str | None = None
    affiliations: list[str] | None = None
    score: float | None = None
    screener_score: float | None = None
    matched_reference: CorpusPaper | None = None
    matched_reference_score: float = 0.0

    def _generate_tldr_with_llm(self, openai_client: OpenAI, llm_params: dict, tokenizer_type: str = "qwen") -> str:
        lang = llm_params.get("language", "English")
        prompt = f"Summarize the paper in 2-3 concise sentences in {lang}, focusing on what it does and its main results or conclusion.\n\n"
        if self.title:
            prompt += f"Title: {self.title}\n\n"

        if self.abstract:
            prompt += f"Abstract: {self.abstract}\n\n"
        elif self.full_text:
            prompt += f"Preview of main content: {self.full_text}\n\n"

        if not self.abstract and not self.full_text:
            logger.warning(f"Neither full text nor abstract is provided for {self.url}")
            return "Failed to generate TLDR. Neither full text nor abstract is provided"

        generation_kwargs = dict(llm_params.get("generation_kwargs", {}))
        max_completion_tokens = generation_kwargs.get("max_tokens", 16384)
        if max_completion_tokens is None or max_completion_tokens > 150:
            generation_kwargs["max_tokens"] = 150

        if tokenizer_type == "qwen":
            enc = _ENC_O200K
        else:
            enc = _ENC_GPT4O

        prompt = prompt.strip().replace("\n\n", "\n")
        prompt_tokens = enc.encode(prompt)
        prompt_tokens = prompt_tokens[:1500]
        prompt = enc.decode(prompt_tokens)

        prompt_token_count = len(prompt_tokens)
        logger.debug(f"TLDR prompt token count for {self.url}: {prompt_token_count}")

        response = openai_client.chat.completions.create(
            messages=[
                {
                    "role": "system",
                    "content": f"You are an assistant that summarizes scientific papers clearly and concisely. Write 2-3 sentences describing the paper's purpose and main result or conclusion in {lang}.",
                },
                {"role": "user", "content": prompt},
            ],
            **generation_kwargs,
        )
        tldr = response.choices[0].message.content

        completion_token_count = count_tokens(tldr, tokenizer_type)
        logger.debug(f"TLDR completion token count for {self.url}: {completion_token_count}")

        add_tldr_tokens(prompt_token_count, completion_token_count)

        return tldr

    def generate_tldr(self, openai_client: OpenAI, llm_params: dict, tokenizer_type: str = "qwen") -> str:
        try:
            tldr = self._generate_tldr_with_llm(openai_client, llm_params, tokenizer_type)
            self.tldr = tldr
            return tldr
        except Exception as e:
            logger.warning(f"Failed to generate tldr of {self.url}: {e}")
            tldr = self.abstract
            self.tldr = tldr
            return tldr

    def _generate_affiliations_with_llm(self, openai_client: OpenAI, llm_params: dict) -> list[str] | None:
        if self.full_text is not None:
            prompt = f"Given the beginning of a paper, extract the affiliations of the authors in a python list format, which is sorted by the author order. If there is no affiliation found, return an empty list '[]':\n\n{self.full_text}"
            enc = _ENC_GPT4O
            prompt_tokens = enc.encode(prompt)
            prompt_tokens = prompt_tokens[:2000]  # truncate to 2000 tokens
            prompt = enc.decode(prompt_tokens)
            affiliations = openai_client.chat.completions.create(
                messages=[
                    {
                        "role": "system",
                        "content": "You are an assistant who perfectly extracts affiliations of authors from a paper. You should return a python list of affiliations sorted by the author order, like [\"TsingHua University\",\"Peking University\"]. If an affiliation is consisted of multi-level affiliations, like 'Department of Computer Science, TsingHua University', you should return the top-level affiliation 'TsingHua University' only. Do not contain duplicated affiliations. If there is no affiliation found, you should return an empty list [ ]. You should only return the final list of affiliations, and do not return any intermediate results.",
                    },
                    {"role": "user", "content": prompt},
                ],
                **llm_params.get("generation_kwargs", {}),
            )
            affiliations = affiliations.choices[0].message.content

            affiliations = re.search(r"\[.*?\]", affiliations, flags=re.DOTALL).group(0)
            affiliations = json.loads(affiliations)
            affiliations = list(set(affiliations))
            affiliations = [str(a) for a in affiliations]

            return affiliations

    def generate_affiliations(self, openai_client: OpenAI, llm_params: dict) -> list[str] | None:
        try:
            affiliations = self._generate_affiliations_with_llm(openai_client, llm_params)
            self.affiliations = affiliations
            return affiliations
        except Exception as e:
            logger.warning(f"Failed to generate affiliations of {self.url}: {e}")
            self.affiliations = None
            return None


@dataclass
class CorpusPaper:
    title: str
    abstract: str
    added_date: datetime
    paths: list[str]
