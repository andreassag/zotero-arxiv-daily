import re
from time import sleep
from typing import Any

import feedparser
import requests
from loguru import logger

from ..protocol import Paper
from .base import BaseRetriever, register_retriever


@register_retriever("biorxiv")
class BiorxivRetriever(BaseRetriever):
    server = "biorxiv"

    def __init__(self, config):
        super().__init__(config)
        if self.retriever_config.category is None:
            raise ValueError(f"category must be specified for {self.name}")

    def _fetch_from_api(self) -> list[dict[str, Any]] | None:
        api_url = f"https://api.biorxiv.org/details/{self.server}/2d"
        headers = {"User-Agent": "Mozilla/5.0 (compatible; zotero-arxiv-daily/1.0)"}
        retry_num = 3
        delay_time = 2
        for i in range(retry_num):
            try:
                response = requests.get(api_url, headers=headers, timeout=15)
                response.raise_for_status()
                text = getattr(response, "text", None)
                if text is not None and not text.strip():
                    raise ValueError("Empty body returned from bioRxiv API")
                result = response.json()
                if not isinstance(result, dict):
                    raise ValueError("Invalid JSON format from bioRxiv API")
                collection = result.get("collection", [])
                if not collection:
                    return []
                all_dates = set(c["date"] for c in collection if "date" in c)
                if not all_dates:
                    return []
                latest_date = sorted(all_dates)[-1]
                collection = [c for c in collection if c.get("date") == latest_date]
                categories = [c.lower() for c in self.retriever_config.category]
                collection = [c for c in collection if c.get("category", "").lower() in categories]
                return collection
            except Exception as e:
                logger.debug(f"bioRxiv JSON API attempt {i + 1} failed: {e}")
                if i < retry_num - 1:
                    sleep(delay_time)
        return None

    def _fetch_from_rss(self) -> list[dict[str, Any]]:
        papers = []
        seen_identifiers = set()
        categories = [c.lower().replace(" ", "_") for c in self.retriever_config.category]
        for cat in categories:
            rss_url = f"https://connect.{self.server}.org/{self.server}_xml.php?subject={cat}"
            try:
                d = feedparser.parse(rss_url)
                for entry in d.entries:
                    title = entry.get("title", "").strip()
                    link = entry.get("link", "").strip()
                    abstract = entry.get("description", "")
                    abstract = re.sub(r"<.*?>", "", abstract).strip()
                    authors = entry.get("dc_creator", entry.get("author", ""))

                    doi_match = re.search(r"10\.\d{4,9}/[-._;()/:A-Za-z0-9]+", link)
                    doi = doi_match.group(0) if doi_match else link

                    identifier = doi or title
                    if identifier in seen_identifiers:
                        continue
                    seen_identifiers.add(identifier)

                    if title and abstract:
                        papers.append(
                            {
                                "title": title,
                                "authors": authors,
                                "abstract": abstract,
                                "doi": doi,
                                "version": "1",
                                "category": cat,
                                "url": link,
                            }
                        )
            except Exception as e:
                logger.warning(f"Failed to fetch {self.server} RSS for {cat}: {e}")
        return papers

    def _retrieve_raw_papers(self) -> list[dict[str, Any]]:
        # 1. Try JSON API first
        papers = self._fetch_from_api()
        if papers is not None:
            if self.config.executor.debug:
                papers = papers[:10]
            return papers

        # 2. Fallback to RSS feed if JSON API is down
        logger.info(f"Using {self.server} RSS feeds as fallback...")
        papers = self._fetch_from_rss()
        if self.config.executor.debug:
            papers = papers[:10]
        return papers


    def convert_to_paper(self, raw_paper: dict[str, Any]) -> Paper | None:
        title = raw_paper["title"]
        authors_raw = raw_paper.get("authors", "")
        if isinstance(authors_raw, list):
            authors = authors_raw
        elif ";" in authors_raw:
            authors = [a.strip() for a in authors_raw.split(";") if a.strip()]
        elif "," in authors_raw:
            authors = [a.strip() for a in authors_raw.split(",") if a.strip()]
        else:
            authors = [authors_raw] if authors_raw else ["Unknown"]

        abstract = raw_paper.get("abstract", "")
        doi = raw_paper.get("doi", "")
        version = raw_paper.get("version", "1")
        if doi and not doi.startswith("http"):
            clean_doi = re.sub(r"v\d+$", "", doi)
            pdf_url = f"https://www.{self.server}.org/content/{clean_doi}v{version}.full.pdf"
        else:
            pdf_url = raw_paper.get("url", "")

        return Paper(
            source=self.name,
            title=title,
            authors=authors,
            abstract=abstract,
            url=raw_paper.get("url") or pdf_url,
            pdf_url=pdf_url,
            full_text=None,
        )


