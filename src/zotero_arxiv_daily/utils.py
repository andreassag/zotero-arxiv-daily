import datetime
import glob
import gzip
import math
import os
import re
import smtplib
import tarfile
from collections import Counter
from email.header import Header
from email.mime.text import MIMEText
from email.utils import formataddr, parseaddr

import pymupdf
import pymupdf.layout
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


def extract_tex_code_from_tar(file_path: str, paper_id: str, paper_title: str | None = None) -> dict[str, str] | None:
    try:
        tar = tarfile.open(file_path)
    except tarfile.ReadError:
        # Attempt to open as a single gzipped .tex file
        try:
            with gzip.open(file_path, "rb") as gz:
                content = gz.read().decode("utf-8", errors="ignore")
                if re.search(r"\\begin\s*\{\s*document\s*\}", content, flags=re.IGNORECASE):
                    content = re.sub(r"%.*\n", "\n", content)
                    content = re.sub(
                        r"\\begin\{comment\}.*?\\end\{comment\}",
                        "",
                        content,
                        flags=re.DOTALL,
                    )
                    content = re.sub(r"\\iffalse.*?\\fi", "", content, flags=re.DOTALL)
                    content = re.sub(r"\n+", "\n", content)
                    content = re.sub(r"\\\\", "", content)
                    content = re.sub(r"[ \t\r\f]{3,}", " ", content)
                    return {"main.tex": content, "all": content}
        except Exception:
            pass
        logger.debug(f"Failed to find main tex file of {paper_id}: Not a tar file.")
        return None

    try:
        tex_files = [f for f in tar.getnames() if f.lower().endswith((".tex", ".ltx"))]
        if len(tex_files) == 0:
            logger.debug(f"Failed to find main tex file of {paper_id}: No tex file.")
            return None

        bbl_file = [f for f in tar.getnames() if f.lower().endswith(".bbl")]
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
                main_name = bbl_file[0][:-4]
                matching_tex = [
                    f
                    for f in tex_files
                    if f.lower() == f"{main_name.lower()}.tex"
                    or re.sub(r"\.[^.]+$", "", os.path.basename(f)).lower()
                    == re.sub(r"\.[^.]+$", "", os.path.basename(main_name)).lower()
                ]
                if matching_tex:
                    main_tex = matching_tex[0]
                else:
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
            if f is None:
                continue
            content = f.read().decode("utf-8", errors="ignore")
            content = re.sub(r"%.*\n", "\n", content)
            content = re.sub(r"\\begin\{comment\}.*?\\end\{comment\}", "", content, flags=re.DOTALL)
            content = re.sub(r"\\iffalse.*?\\fi", "", content, flags=re.DOTALL)
            content = re.sub(r"\n+", "\n", content)
            content = re.sub(r"\\\\", "", content)
            content = re.sub(r"[ \t\r\f]{3,}", " ", content)
            if (
                main_tex is None
                and re.search(r"\\begin\s*\{\s*document\s*\}", content, flags=re.IGNORECASE)
                and not any(w in t.lower() for w in ["example", "sample", "template"])
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
            main_source: str = file_contents.get(main_tex, "")
            # find and replace all included sub-files
            include_files = re.findall(r"\\input\{(.+?)\}", main_source) + re.findall(
                r"\\include\{(.+?)\}", main_source
            )
            for f in include_files:
                f_clean = f.strip().strip('"').strip("'")
                candidates_to_try = [
                    f_clean,
                    f_clean + ".tex",
                    os.path.basename(f_clean),
                    os.path.basename(f_clean) + ".tex",
                ]
                sub_content = ""
                for cand in candidates_to_try:
                    for k, v in file_contents.items():
                        if k == cand or os.path.basename(k) == cand or k.endswith("/" + cand):
                            sub_content = v
                            break
                    if sub_content:
                        break
                main_source = main_source.replace(f"\\input{{{f}}}", sub_content).replace(
                    f"\\include{{{f}}}", sub_content
                )
            file_contents["all"] = main_source
        else:
            logger.debug(f"Failed to find main tex file of {paper_id}: No tex file containing the document block.")
            file_contents["all"] = None

        return file_contents
    finally:
        tar.close()


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
    smtp_username = config.email.get("smtp_username", config.email.get("smtp_user", sender))

    def _format_addr(s):
        name, addr = parseaddr(s)
        return formataddr((Header(name, "utf-8").encode(), addr))

    msg = MIMEText(html, "html", "utf-8")
    msg["From"] = _format_addr("Github Action <%s>" % sender)
    msg["To"] = _format_addr("You <%s>" % receiver)
    today = datetime.datetime.now().strftime("%Y/%m/%d")
    msg["Subject"] = Header(f"Daily arXiv {today}", "utf-8").encode()

    if int(smtp_port) == 465:
        try:
            server = smtplib.SMTP_SSL(smtp_server, smtp_port, timeout=30)
        except Exception as e:
            logger.debug(f"Failed to use SSL on port {smtp_port}: {e}. Trying TLS.")
            server = smtplib.SMTP(smtp_server, smtp_port, timeout=30)
            server.starttls()
    else:
        try:
            server = smtplib.SMTP(smtp_server, smtp_port, timeout=30)
            server.starttls()
        except Exception as e:
            logger.debug(f"Failed to use TLS. {e}\nTry to use SSL.")
            try:
                server = smtplib.SMTP_SSL(smtp_server, smtp_port, timeout=30)
            except Exception as e:
                logger.debug(f"Failed to use SSL. {e}\nTry to use plain text.")
                server = smtplib.SMTP(smtp_server, smtp_port, timeout=30)

    server.login(smtp_username, password)
    server.sendmail(sender, [receiver], msg.as_string())
    server.quit()
