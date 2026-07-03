import datetime
import glob
import math
import re
import smtplib
import tarfile
from collections import Counter
from email.header import Header
from email.mime.text import MIMEText
from email.utils import formataddr, parseaddr

import pymupdf
import pymupdf.layout
import tiktoken
from loguru import logger
from omegaconf import DictConfig

pymupdf.TOOLS.mupdf_display_errors(False)
pymupdf.layout.activate()

import pymupdf4llm  # noqa: E402

_TOKEN_RE = re.compile(r"[a-zA-Z0-9]+")


def _tokenize(text: str) -> list[str]:
    return [t.lower() for t in _TOKEN_RE.findall(text)]


def _bm25_pick(query: str, candidates: dict[str, str], k1: float = 1.5, b: float = 0.75) -> str:
    """Return the candidate key whose content best matches *query* by BM25."""
    query_tokens = _tokenize(query)
    if not query_tokens:
        return next(iter(candidates))

    doc_tokens = {name: _tokenize(content) for name, content in candidates.items()}
    N = len(doc_tokens)
    avgdl = sum(len(t) for t in doc_tokens.values()) / max(N, 1)

    df: Counter[str] = Counter()
    for tokens in doc_tokens.values():
        df.update(set(tokens))

    best_name, best_score = None, -1.0
    for name, tokens in doc_tokens.items():
        tf = Counter(tokens)
        dl = len(tokens)
        score = 0.0
        for q in query_tokens:
            n_q = df.get(q, 0)
            idf = math.log((N - n_q + 0.5) / (n_q + 0.5) + 1)
            f_q = tf.get(q, 0)
            score += idf * (f_q * (k1 + 1)) / (f_q + k1 * (1 - b + b * dl / max(avgdl, 1)))
        if score > best_score:
            best_score = score
            best_name = name
    return best_name


def extract_tex_code_from_tar(file_path: str, paper_id: str, paper_title: str | None = None) -> dict[str, str]:
    try:
        tar = tarfile.open(file_path)
    except tarfile.ReadError:
        logger.debug(f"Failed to find main tex file of {paper_id}: Not a tar file.")
        return None

    tex_files = [f for f in tar.getnames() if f.endswith(".tex")]
    if len(tex_files) == 0:
        logger.debug(f"Failed to find main tex file of {paper_id}: No tex file.")
        tar.close()
        return None

    bbl_file = [f for f in tar.getnames() if f.endswith(".bbl")]
    match len(bbl_file):
        case 0:
            if len(tex_files) > 1:
                logger.debug(
                    f"Cannot find main tex file of {paper_id} from bbl: There are multiple tex files while no bbl file."
                )
                main_tex = None
            else:
                main_tex = tex_files[0]
        case 1:
            main_name = bbl_file[0].replace(".bbl", "")
            main_tex = f"{main_name}.tex"
            if main_tex not in tex_files:
                logger.debug(
                    f"Cannot find main tex file of {paper_id} from bbl: The bbl file does not match any tex file."
                )
                main_tex = None
        case _:
            logger.debug(f"Cannot find main tex file of {paper_id} from bbl: There are multiple bbl files.")
            main_tex = None

    if main_tex is None:
        logger.debug(f"Trying to choose tex file containing the document block as main tex file of {paper_id}")

    file_contents = {}
    doc_block_candidates: list[str] = []
    for t in tex_files:
        f = tar.extractfile(t)
        content = f.read().decode("utf-8", errors="ignore")
        content = re.sub(r"%.*\n", "\n", content)
        content = re.sub(r"\\begin{comment}.*?\\end{comment}", "", content, flags=re.DOTALL)
        content = re.sub(r"\\iffalse.*?\\fi", "", content, flags=re.DOTALL)
        content = re.sub(r"\n+", "\n", content)
        content = re.sub(r"\\\\", "", content)
        content = re.sub(r"[ \t\r\f]{3,}", " ", content)
        if (
            main_tex is None
            and re.search(r"\\begin\{document\}", content)
            and not any(w in t for w in ["example", "sample", "template"])
        ):
            doc_block_candidates.append(t)
        file_contents[t] = content

    if main_tex is None:
        if len(doc_block_candidates) == 1:
            main_tex = doc_block_candidates[0]
            logger.debug(f"Choose {main_tex} as main tex file of {paper_id}")
        elif len(doc_block_candidates) > 1:
            if paper_title:
                main_tex = _bm25_pick(paper_title, {c: file_contents[c] for c in doc_block_candidates})
                logger.debug(
                    f"Multiple document blocks found in {paper_id}; BM25 selected {main_tex} from {doc_block_candidates}"
                )
            else:
                main_tex = doc_block_candidates[0]
                logger.debug(
                    f"Multiple document blocks found in {paper_id}; no title provided, using first candidate {main_tex}"
                )

    if main_tex is not None:
        main_source: str = file_contents[main_tex]
        # find and replace all included sub-files
        include_files = re.findall(r"\\input\{(.+?)\}", main_source) + re.findall(r"\\include\{(.+?)\}", main_source)
        for f in include_files:
            if not f.endswith(".tex"):
                file_name = f + ".tex"
            else:
                file_name = f
            main_source = main_source.replace(f"\\input{{{f}}}", file_contents.get(file_name, ""))
        file_contents["all"] = main_source
    else:
        logger.debug(f"Failed to find main tex file of {paper_id}: No tex file containing the document block.")
        file_contents["all"] = None

    tar.close()
    return file_contents


def extract_markdown_from_pdf(file_path: str) -> str:
    return pymupdf4llm.to_markdown(file_path, use_ocr=False, header=False, footer=False, ignore_code=True)


def glob_match(path: str, pattern: str) -> bool:
    re_pattern = glob.translate(pattern, recursive=True)
    return re.match(re_pattern, path) is not None


def send_email(config: DictConfig, html: str):
    sender = config.email.sender
    receiver = config.email.receiver
    password = config.email.sender_password
    smtp_server = config.email.smtp_server
    smtp_port = config.email.smtp_port
    smtp_user = config.email.get("smtp_user", sender)

    def _format_addr(s):
        name, addr = parseaddr(s)
        return formataddr((Header(name, "utf-8").encode(), addr))

    msg = MIMEText(html, "html", "utf-8")
    msg["From"] = _format_addr("Github Action <%s>" % sender)
    msg["To"] = _format_addr("You <%s>" % receiver)
    today = datetime.datetime.now().strftime("%Y/%m/%d")
    msg["Subject"] = Header(f"Daily arXiv {today}", "utf-8").encode()

    try:
        server = smtplib.SMTP(smtp_server, smtp_port)
        server.starttls()
    except Exception as e:
        logger.debug(f"Failed to use TLS. {e}\nTry to use SSL.")
        try:
            server = smtplib.SMTP_SSL(smtp_server, smtp_port)
        except Exception as e:
            logger.debug(f"Failed to use SSL. {e}\nTry to use plain text.")
            server = smtplib.SMTP(smtp_server, smtp_port)

    server.login(smtp_user, password)
    server.sendmail(sender, [receiver], msg.as_string())
    server.quit()


# Global token usage tracking
_token_usage_stats = {"embedding_tokens": 0, "tldr_prompt_tokens": 0, "tldr_completion_tokens": 0, "total_tokens": 0}


def get_token_usage_stats():
    """Get current token usage statistics."""
    return _token_usage_stats.copy()


def reset_token_usage_stats():
    """Reset token usage statistics."""
    global _token_usage_stats
    _token_usage_stats = {
        "embedding_tokens": 0,
        "tldr_prompt_tokens": 0,
        "tldr_completion_tokens": 0,
        "total_tokens": 0,
    }


def print_token_usage_stats():
    """Print current token usage statistics."""
    stats = get_token_usage_stats()
    logger.info("Token Usage Statistics:")
    logger.info(f"  Embedding tokens: {stats['embedding_tokens']}")
    logger.info(f"  TLDR prompt tokens: {stats['tldr_prompt_tokens']}")
    logger.info(f"  TLDR completion tokens: {stats['tldr_completion_tokens']}")
    logger.info(f"  Total tokens: {stats['total_tokens']}")


def _get_qwen_tokenizer():
    """Get Qwen tokenizer, using official Qwen tokenizer or closest approximation."""
    try:
        # Try to use official Qwen tokenizer via transformers
        from transformers import AutoTokenizer

        try:
            tokenizer = AutoTokenizer.from_pretrained("Qwen/Qwen2-7B-Instruct")
            logger.debug("Using official Qwen tokenizer")
            return tokenizer
        except Exception as e:
            logger.debug(f"Failed to load official Qwen tokenizer: {e}")
            # Fall back to o200k_base which is closest to Qwen
            logger.debug("Falling back to o200k_base tiktoken encoding")
            return tiktoken.get_encoding("o200k_base")
    except ImportError:
        # Fall back to tiktoken o200k_base
        logger.debug("Transformers not available, using o200k_base tiktoken encoding")
        return tiktoken.get_encoding("o200k_base")


def count_tokens(text: str, tokenizer_type: str = "qwen") -> int:
    """
    Count tokens in text using specified tokenizer.

    Args:
        text: Input text to tokenize
        tokenizer_type: Type of tokenizer to use. Options: "qwen", "gpt4", "cl100k_base", "o200k_base"

    Returns:
        Number of tokens
    """
    if tokenizer_type == "qwen":
        tokenizer = _get_qwen_tokenizer()
        if hasattr(tokenizer, "encode"):
            # tiktoken encoder
            return len(tokenizer.encode(text))
        else:
            # transformers tokenizer
            return len(tokenizer.encode(text))
    else:
        # Use tiktoken for other encodings
        try:
            if tokenizer_type == "gpt4":
                encoding = tiktoken.encoding_for_model("gpt-4o")
            else:
                encoding = tiktoken.get_encoding(tokenizer_type)
            return len(encoding.encode(text))
        except Exception as e:
            logger.warning(f"Failed to get tokenizer {tokenizer_type}: {e}")
            # Fallback to o200k_base
            encoding = tiktoken.get_encoding("o200k_base")
            return len(encoding.encode(text))


def count_tokens_batch(texts: list[str], tokenizer_type: str = "qwen") -> list[int]:
    """
    Count tokens for a batch of texts.

    Args:
        texts: List of input texts
        tokenizer_type: Type of tokenizer to use

    Returns:
        List of token counts corresponding to each text
    """
    return [count_tokens(text, tokenizer_type) for text in texts]


def add_embedding_tokens(token_count: int):
    """Add token count to embedding token statistics."""
    _token_usage_stats["embedding_tokens"] += token_count
    _token_usage_stats["total_tokens"] += token_count


def add_tldr_tokens(prompt_tokens: int, completion_tokens: int):
    """Add token counts to TLDR token statistics."""
    _token_usage_stats["tldr_prompt_tokens"] += prompt_tokens
    _token_usage_stats["tldr_completion_tokens"] += completion_tokens
    _token_usage_stats["total_tokens"] += prompt_tokens + completion_tokens
