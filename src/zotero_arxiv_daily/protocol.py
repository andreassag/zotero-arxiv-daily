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

    def _generate_tldr_with_llm(self, openai_client: OpenAI, llm_params: dict, tokenizer_type: str = "qwen") -> str:
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

        # Count prompt tokens using the specified tokenizer
        prompt_token_count = count_tokens(prompt, tokenizer_type)
        logger.debug(f"TLDR prompt token count for {self.url}: {prompt_token_count}")

        # Truncate prompt if it's too long (keep within reasonable limits)
        if tokenizer_type == "qwen":
            # Use tiktoken o200k_base as approximation for truncation
            enc = tiktoken.get_encoding("o200k_base")
        else:
            enc = tiktoken.encoding_for_model("gpt-4o")

        prompt_tokens = enc.encode(prompt)
        prompt_tokens = prompt_tokens[:4000]  # truncate to 4000 tokens
        prompt = enc.decode(prompt_tokens)

        response = openai_client.chat.completions.create(
            messages=[
                {
                    "role": "system",
                    "content": f"You are an assistant who perfectly summarizes scientific paper, and gives the core idea of the paper to the user. Your answer should be in {lang}.",
                },
                {"role": "user", "content": prompt},
            ],
            **llm_params.get("generation_kwargs", {}),
        )
        tldr = response.choices[0].message.content

        # Count completion tokens
        completion_token_count = count_tokens(tldr, tokenizer_type)
        logger.debug(f"TLDR completion token count for {self.url}: {completion_token_count}")

        # Track token usage
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
            # use gpt-4o tokenizer for estimation
            enc = tiktoken.encoding_for_model("gpt-4o")
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
