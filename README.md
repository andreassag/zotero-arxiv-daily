<p align="center">
  <a href="" rel="noopener">
 <img width=200px height=200px src="assets/logo.svg" alt="logo"></a>
</p>

<h3 align="center">Zotero-arXiv-Daily</h3>

<div align="center">

  [![Status](https://img.shields.io/badge/status-active-success.svg)]()
  ![Stars](https://img.shields.io/github/stars/TideDra/zotero-arxiv-daily?style=flat)
  [![GitHub Issues](https://img.shields.io/github/issues/TideDra/zotero-arxiv-daily)](https://github.com/TideDra/zotero-arxiv-daily/issues)
  [![GitHub Pull Requests](https://img.shields.io/github/issues-pr/TideDra/zotero-arxiv-daily)](https://github.com/TideDra/zotero-arxiv-daily/pulls)
  [![License](https://img.shields.io/github/license/TideDra/zotero-arxiv-daily)](/LICENSE)
  [<img src="https://api.gitsponsors.com/api/badge/img?id=893025857" height="20">](https://api.gitsponsors.com/api/badge/link?p=PKMtRut1dWWuC1oFdJweyDSvJg454/GkdIx4IinvBblaX2AY4rQ7FYKAK1ZjApoiNhYEeduIEhfeZVIwoIVlvcwdJXVFD2nV2EE5j6lYXaT/RHrcsQbFl3aKe1F3hliP26OMayXOoZVDidl05wj+yg==)

</div>

---

<p align="center"> Recommend new arxiv papers of your interest daily according to your Zotero library.
    <br> 
</p>

> [!IMPORTANT]
> Please keep an eye on this repo, and merge your forked repo in time when there is any update of this upstream, in order to enjoy new features and fix found bugs.

## 🧐 About <a name = "about"></a>

> Track new scientific researches of your interest by just forking (and staring) this repo!😊

*Zotero-arXiv-Daily* finds arxiv papers that may attract you based on the context of your Zotero library, and then sends the result to your mailbox📮. It can be deployed as Github Action Workflow with **zero cost**, **no installation**, and **few configuration** of Github Action environment variables for daily **automatic** delivery.

## ✨ Features
- Totally free! All the calculation can be done in the Github Action runner locally within its quota (for public repo).
- AI-generated TL;DR for you to quickly pick up target papers.
- Affiliations of the paper are resolved and presented.
- Links of PDF and code implementation (if any) presented in the e-mail.
- List of papers sorted by relevance with your recent research interest.
- Fast deployment via fork this repo and set environment variables in the Github Action Page.
- Support LLM API for generating TL;DR of papers.
- Ignore unwanted Zotero papers using a list of glob patterns.
- Support multiple sources of papers to retrieve:
  - arxiv
  - biorxiv
  - medrxiv

## 📷 Screenshot
![screenshot](./assets/screenshot.png)

## 🚀 Usage
### Quick Start
1. Fork (and star😘) this repo.
![fork](./assets/fork.png)

2. Set Github Action environment variables.
![secrets](./assets/secrets.png)

Below are the secret keys you can configure in GitHub Actions or in your `.env` file:

| Key | Description | Example |
| :--- | :--- | :--- |
| `ZOTERO_ID` | User ID of your Zotero account (a sequence of numbers from [Zotero Settings](https://www.zotero.org/settings/keys)). | `12345678` |
| `ZOTERO_KEY` | A Zotero API key with library read access from [Zotero Settings](https://www.zotero.org/settings/keys). | `AB5tZ877P2j7Sm2Mragq041H` |
| `SMTP_SERVER` | The SMTP server hostname. | `smtp.gmail.com` or `smtp.tem.scaleway.com` |
| `SMTP_PORT` | The SMTP port (465 for SSL, 587 for TLS). | `465` |
| `SMTP_USER` | The SMTP authentication username (often same as sender). | `sender@example.com` |
| `SMTP_PASSWORD` | The SMTP password or application password. | `abcdefghijklmn` |
| `SMTP_SENDER` | The sender email address. | `sender@example.com` |
| `SMTP_RECEIVER` | The email address that receives the daily paper digest. | `receiver@example.com` |
| `LLM_API_KEY` | API Key for LLM summarization. Get a FREE API key from [Google AI Studio](https://aistudio.google.com/app/apikey). | `AIzaSy...` |
| `LLM_BASE_URL` | Base URL of LLM API. Defaults to Google Gemini OpenAI-compatible endpoint. | `https://generativelanguage.googleapis.com/v1beta/openai/` |
| `LLM_MODEL` | LLM model identifier. Defaults to `gemini-2.5-flash`. | `gemini-2.5-flash` |

All options in `config/` can be configured directly through `.env` environment variables without editing YAML files. You can optionally set a public GitHub Actions variable `CUSTOM_CONFIG` to override settings:
![vars](./assets/repo_var.png)
![custom_config](./assets/config_var.png)

```yaml
zotero:
  user_id: ${oc.env:ZOTERO_ID}
  api_key: ${oc.env:ZOTERO_KEY}
  include_path: ${oc.decode:${oc.env:ZOTERO_INCLUDE_PATH,null}}
  ignore_path: ${oc.decode:${oc.env:ZOTERO_IGNORE_PATH,null}}

email:
  sender: ${oc.env:SMTP_SENDER}
  receiver: ${oc.env:SMTP_RECEIVER}
  smtp_server: ${oc.env:SMTP_SERVER}
  smtp_port: ${oc.decode:${oc.env:SMTP_PORT,465}}
  sender_password: ${oc.env:SMTP_PASSWORD}
  smtp_user: ${oc.env:SMTP_USER,${email.sender}}

llm:
  api:
    key: ${oc.env:LLM_API_KEY}
    base_url: ${oc.env:LLM_BASE_URL,https://generativelanguage.googleapis.com/v1beta/openai/}
  generation_kwargs:
    model: ${oc.env:LLM_MODEL,gemini-2.5-flash}
    max_tokens: ${oc.decode:${oc.env:LLM_MAX_TOKENS,16384}}
  language: ${oc.env:LLM_LANGUAGE,English}

source:
  arxiv:
    category: ${oc.decode:${oc.env:ARXIV_CATEGORIES,['cs.AI','cs.CV','cs.LG','cs.CL']}}
    include_cross_list: ${oc.decode:${oc.env:INCLUDE_CROSS_LIST,false}}

executor:
  debug: ${oc.decode:${oc.env:DEBUG,false}}
  max_paper_num: ${oc.decode:${oc.env:MAX_PAPER_NUM,100}}
  source: ${oc.decode:${oc.env:EXECUTOR_SOURCES,['arxiv']}}
  reranker: ${oc.env:RERANKER_TYPE,local}
```
Set `source.arxiv.include_cross_list: true` if you want cross-listed papers included.
>[!NOTE]
> `${oc.env:XXX,yyy}` means the value of the environment variable `XXX`. If the variable is not set, the default value `yyy` will be used.

Here is the full configuration, `???` means the value must be filled in:
```yaml
zotero:
  user_id: ??? # User ID of your Zotero account.
  api_key: ??? # A Zotero API key with read access.
  include_path: null # A list of glob patterns marking the Zotero collections that should be included. Example: ["2026/survey/**", "2026/reading-group/**"]

source:
  arxiv:
    category: null # The categories of target arxiv papers. Example: ["cs.AI","cs.CV","cs.LG","cs.CL"]
    include_cross_list: false # Whether to include arXiv cross-list papers in subscribed categories. Example: true
  biorxiv:
    category: null # The categories of target biorxiv papers. Example: ["biochemistry","animal behavior and cognition"]
  medrxiv:
    category: null # The categories of target medrxiv papers. Example: ["psychiatry and clinical psychology", "neurology"]

email:
  sender: ??? # The email address that sends the digest. Example: sender@example.com
  receiver: ??? # The email address that receives the paper list. Example: receiver@example.com
  smtp_server: ??? # The SMTP server address. Example: smtp.gmail.com or smtp.tem.scaleway.com
  smtp_port: 465 # The port of SMTP server. Example: 465 or 587
  sender_password: ??? # The password or app token for the SMTP service.
  smtp_user: ${email.sender} # Optional SMTP login username. Defaults to sender if not explicitly set.

llm:
  api:
    key: ??? # API Key of your LLM provider.
    base_url: https://generativelanguage.googleapis.com/v1beta/openai/ # API base URL of LLM API.
  generation_kwargs:
    max_tokens: 16384
    model: gemini-2.5-flash # Model name. Example: gemini-2.5-flash, gemini-2.0-flash, gpt-4o-mini
  language: English # Preferred language for the TL;DR. Example: English
  rate_limit:
    rpm: 15
    tpm: 250000
    rpd: 500

reranker:
  api:
    key: ${llm.api.key} # API Key for embedding model (defaults to LLM API key).
    base_url: ${llm.api.base_url} # API base URL for embedding endpoint.
    model: text-embedding-004 # The model name of the embedding model. Example: text-embedding-004
    batch_size: 32 # The batch size for embedding API requests.
  rate_limit:
    rpm: 100
    tpm: 30000
    rpd: 1000

executor:
  debug: false # Whether to use debug mode. Example: true
  send_empty: false # Whether to send an empty email even if no new papers today. Example: true
  max_paper_num: 100 # The maximum number of papers presented in the email. Example: 100
  source: ["arxiv"] # The sources of papers to retrieve. Example: ['arxiv']
```

That's all! Now you can test the workflow by manually triggering it:
![test](./assets/test.png)

> [!NOTE]
> The Test-Workflow Action is the debug version of the main workflow (Send-emails-daily), which always retrieve 5 arxiv papers regardless of the date. While the main workflow will be automatically triggered everyday and retrieve new papers released yesterday. There is no new arxiv paper at weekends and holiday, in which case you may see "No new papers found" in the log of main workflow.

Then check the log and the receiver email after it finishes.

By default, the main workflow runs on 22:00 UTC everyday. You can change this time by editting the workflow config `.github/workflows/main.yml`.

### Local Running
Supported by [uv](https://github.com/astral-sh/uv), this workflow can easily run on your local device if uv is installed:
```bash
# set all the environment variables
# export ZOTERO_ID=xxxx
# ...
cd zotero-arxiv-daily
uv run main.py
```

## 🚀 Sync with the latest version
This project is in active development. You can subscribe this repo via `Watch` so that you can be notified once we publish new release.

![Watch](./assets/subscribe_release.png)


## 📖 How it works
*Zotero-arXiv-Daily* firstly retrieves all the papers in your Zotero library and all the papers released in the previous day, via corresponding API. Then it calculates the embedding of each paper's abstract via an embedding model. The score of a paper is its weighted average similarity over all your Zotero papers (newer paper added to the library has higher weight). The TLDR of each paper is generated by LLM, given the text extracted by pymupdf4llm.

## 📌 Limitations
- The recommendation algorithm is very simple, it may not accurately reflect your interest. Welcome better ideas for improving the algorithm!
- High `MAX_PAPER_NUM` can lead the execution time exceed the limitation of Github Action runner (6h per execution for public repo, and 2000 mins per month for private repo). Commonly, the quota given to public repo is definitely enough for individual use. If you have special requirements, you can deploy the workflow in your own server, or use a self-hosted Github Action runner, or pay for the exceeded execution time.


## 📃 License
Distributed under the AGPLv3 License. See `LICENSE` for detail.

## ❤️ Acknowledgement
- [pyzotero](https://github.com/urschrei/pyzotero)
- [arxiv](https://github.com/lukasschwab/arxiv.py)
- [sentence_transformers](https://github.com/UKPLab/sentence-transformers)

## ☕ Buy Me A Coffee
If you find this project helpful, welcome to sponsor me via WeChat or via [ko-fi](https://ko-fi.com/tidedra).
![wechat_qr](assets/wechat_sponsor.JPG)


## 🌟 Star History

[![Star History Chart](https://api.star-history.com/svg?repos=TideDra/zotero-arxiv-daily&type=Date)](https://star-history.com/#TideDra/zotero-arxiv-daily&Date)
