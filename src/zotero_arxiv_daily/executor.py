import os
import random
from concurrent.futures import ThreadPoolExecutor, as_completed
from datetime import datetime

from loguru import logger
from omegaconf import DictConfig, ListConfig
from openai import OpenAI
from pyzotero import zotero
from tqdm import tqdm

from .construct_email import render_email
from .protocol import CorpusPaper
from .reranker import get_cross_encoder_reranker, get_screener_cls
from .retriever import get_retriever_cls
from .utils import glob_match, print_token_usage_stats, reset_token_usage_stats, send_email


def normalize_path_patterns(patterns: list[str] | ListConfig | None, config_key: str) -> list[str] | None:
    if patterns is None:
        return None

    if not isinstance(patterns, (list, ListConfig)):
        raise TypeError(
            f"config.zotero.{config_key} must be a list of glob patterns or null, "
            'for example ["2026/survey/**"]. Single strings are not supported.'
        )

    if any(not isinstance(pattern, str) for pattern in patterns):
        raise TypeError(f"config.zotero.{config_key} must contain only glob pattern strings.")

    return list(patterns)


class Executor:
    def __init__(self, config: DictConfig):
        self.config = config
        self.include_path_patterns = normalize_path_patterns(
            getattr(config.zotero, "include_path", None), "include_path"
        )
        self.ignore_path_patterns = normalize_path_patterns(getattr(config.zotero, "ignore_path", None), "ignore_path")

        # Resolve active paper sources (auto-detect if executor.source is null/omitted)
        exec_sources = getattr(config.executor, "source", None)
        if exec_sources:
            active_sources = list(exec_sources)
        else:
            active_sources = [k for k, v in config.source.items() if getattr(v, "category", None) is not None] or list(
                config.source.keys()
            )

        self.retrievers = {source: get_retriever_cls(source)(config) for source in active_sources}

        # Resolve Stage 1 Screener and Stage 2 Reranker
        self.screener = get_screener_cls("api")(config)
        self.reranker = self.screener  # Backward compatibility reference
        self.stage2_reranker = get_cross_encoder_reranker(config)

        self.openai_client = OpenAI(api_key=config.llm.api.key, base_url=config.llm.api.base_url)

    def fetch_zotero_corpus(self) -> list[CorpusPaper]:
        logger.info("Fetching zotero corpus")
        zot = zotero.Zotero(self.config.zotero.user_id, "user", self.config.zotero.api_key)
        collections = zot.everything(zot.collections())
        collections = {c["key"]: c for c in collections}
        corpus = zot.everything(zot.items(itemType="conferencePaper || journalArticle || preprint"))
        corpus = [c for c in corpus if c["data"]["abstractNote"] != ""]

        def get_collection_path(col_key: str) -> str:
            if p := collections[col_key]["data"]["parentCollection"]:
                return get_collection_path(p) + "/" + collections[col_key]["data"]["name"]
            else:
                return collections[col_key]["data"]["name"]

        for c in corpus:
            paths = [get_collection_path(col) for col in c["data"]["collections"]]
            c["paths"] = paths
        logger.info(f"Fetched {len(corpus)} zotero papers")
        return [
            CorpusPaper(
                title=c["data"]["title"],
                abstract=c["data"]["abstractNote"],
                added_date=datetime.strptime(c["data"]["dateAdded"], "%Y-%m-%dT%H:%M:%SZ"),
                paths=c["paths"],
            )
            for c in corpus
        ]

    def filter_corpus(self, corpus: list[CorpusPaper]) -> list[CorpusPaper]:
        if self.include_path_patterns:
            logger.info(f"Selecting zotero papers matching include_path: {self.include_path_patterns}")
            corpus = [
                c
                for c in corpus
                if any(glob_match(path, pattern) for path in c.paths for pattern in self.include_path_patterns)
            ]
        if self.ignore_path_patterns:
            logger.info(f"Excluding zotero papers matching ignore_path: {self.ignore_path_patterns}")
            corpus = [
                c
                for c in corpus
                if not any(glob_match(path, pattern) for path in c.paths for pattern in self.ignore_path_patterns)
            ]
        if self.include_path_patterns or self.ignore_path_patterns:
            samples = random.sample(corpus, min(5, len(corpus)))
            samples = "\n".join([c.title + " - " + "\n".join(c.paths) for c in samples])
            logger.info(f"Selected {len(corpus)} zotero papers:\n{samples}\n...")
        return corpus

    def run(self):
        # Reset token usage statistics at the beginning of each run
        reset_token_usage_stats()

        corpus = self.fetch_zotero_corpus()
        corpus = self.filter_corpus(corpus)
        if len(corpus) == 0:
            logger.error(f"No zotero papers found. Please check your zotero settings:\n{self.config.zotero}")
            if self.config.executor.debug:
                print_token_usage_stats()
            return
        all_papers = []
        for source, retriever in self.retrievers.items():
            logger.info(f"Retrieving {source} papers...")
            papers = retriever.retrieve_papers()
            if len(papers) == 0:
                logger.info(f"No {source} papers found")
                continue
            logger.info(f"Retrieved {len(papers)} {source} papers")
            all_papers.extend(papers)
        logger.info(f"Total {len(all_papers)} papers retrieved from all sources")

        reranked_papers = []
        if len(all_papers) > 0:
            screen_top_k = getattr(getattr(self.config, "screener", {}), "screen_top_k", 50)
            logger.info(f"Stage 1: Screening {len(all_papers)} preprints...")
            screened_papers = self.screener.screen(all_papers, corpus, top_k=screen_top_k)
            logger.info(f"Stage 1 completed: {len(screened_papers)} candidates selected")

            if self.stage2_reranker is not None:
                logger.info("Stage 2: Cross-encoder reranking candidates...")
                reranked_papers = self.stage2_reranker.rerank(screened_papers, corpus)
            else:
                reranked_papers = screened_papers

            max_num = getattr(getattr(self.config, "reranker", {}), "top_k", self.config.executor.max_paper_num)
            reranked_papers = reranked_papers[:max_num]

            logger.info("Generating TLDR and affiliations in parallel...")
            tokenizer_type = self.config.executor.get("tokenizer_type", "qwen")

            def _process_paper_metadata(paper):
                paper.generate_tldr(self.openai_client, self.config.llm, tokenizer_type)
                paper.generate_affiliations(self.openai_client, self.config.llm)
                return paper

            with ThreadPoolExecutor(max_workers=8) as pool:
                futures = {pool.submit(_process_paper_metadata, p): p for p in reranked_papers}
                for f in tqdm(as_completed(futures), total=len(futures), desc="Generating metadata"):
                    try:
                        f.result()
                    except Exception as exc:
                        p = futures[f]
                        logger.warning(f"Metadata generation failed for {p.url}: {exc}")

        elif not self.config.executor.send_empty:
            logger.info("No new papers found. No email will be sent.")
            if self.config.executor.debug:
                print_token_usage_stats()
            return
        # Check if email sending should be skipped
        no_email = os.environ.get("NO_EMAIL", "").lower() in ("true", "1", "yes")
        email_content = render_email(reranked_papers)

        # Save HTML locally if SAVE_EMAIL_PATH is set, or automatically when NO_EMAIL/debug mode is active
        save_path = os.environ.get("SAVE_EMAIL_PATH") or (
            "email_output.html" if (no_email or self.config.executor.debug) else None
        )
        if save_path:
            try:
                with open(save_path, "w", encoding="utf-8") as f:
                    f.write(email_content)
                logger.info(f"Generated HTML saved locally to: {save_path}")
            except Exception as e:
                logger.error(f"Failed to save email content to {save_path}: {e}")

        if no_email:
            logger.info("NO_EMAIL environment variable set. Skipping email sending.")
            logger.info("Email content preview:")
            logger.info(email_content[:500] + "..." if len(email_content) > 500 else email_content)
        else:
            logger.info("Sending email...")
            send_email(self.config, email_content)
            logger.info("Email sent successfully")

        # Print token usage statistics at the end (only in debug mode)
        if self.config.executor.debug:
            print_token_usage_stats()
