// Recommend new arxiv papers of your interest daily according to your Zotero library.
// Copyright (C) 2026  Andreas Sagen

// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.

// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/exTerEX/zotero-arxiv-daily/go/internal/llm"
	"github.com/exTerEX/zotero-arxiv-daily/go/internal/model"
)

var (
	// HighWire Press meta tags (used by bioRxiv, medRxiv, and many academic journals)
	metaTagRegex     = regexp.MustCompile(`(?i)<meta\s+([^>]+)>`)
	metaNameRegex    = regexp.MustCompile(`(?i)\bname\s*=\s*["'](?:citation_author_institution|institution|dc\.publisher)["']`)
	metaContentRegex = regexp.MustCompile(`(?i)\bcontent\s*=\s*["']([^"']*)["']`)

	// arXiv HTML affiliations (<span class="ltx_contact ltx_role_affiliation">)
	arxivAffiliationRegex = regexp.MustCompile(`(?i)class=["'][^"']*ltx_role_affiliation[^"']*["'][^>]*>(?:<span[^>]*>[^<]*</span>)?([^<]+)`)

	arxivPathRegex = regexp.MustCompile(`^/(?:abs|pdf|html)/(\d{4}\.\d{4,5}(?:v\d+)?)(?:\.pdf)?$`)
	doiRegex       = regexp.MustCompile(`\b(10\.\d{4,9}/[-._;()/:A-Za-z0-9]+)`)
)

type openAlexInstitution struct {
	DisplayName string `json:"display_name"`
}

type openAlexAuthorship struct {
	Institutions          []openAlexInstitution `json:"institutions"`
	RawAffiliationStrings []string              `json:"raw_affiliation_strings"`
}

type openAlexWork struct {
	Authorships []openAlexAuthorship `json:"authorships"`
}

type openAlexResponse struct {
	Results []openAlexWork `json:"results"`
}

// ExtractAffiliations determines author affiliations for a paper.
// 1. Attempts direct extraction from the paper's web page metadata (fast, authoritative, 0 LLM tokens).
// 2. Attempts resolution via the OpenAlex scholarly database API.
// 3. Falls back to LLM extraction if previous attempts yield no affiliations.
func ExtractAffiliations(ctx context.Context, httpClient *http.Client, llmClient *llm.Client, p *model.Paper, modelName string) []string {
	// 1. Try web page extraction
	if p.URL != "" || p.PDFURL != "" {
		affs, err := extractFromURL(ctx, httpClient, p.URL, p.PDFURL)
		if err == nil && len(affs) > 0 {
			slog.Debug("Extracted affiliations from HTML metadata", "paper", p.Title, "count", len(affs))
			return affs
		}
	}

	// 2. Try OpenAlex lookup
	if p.Title != "" || p.URL != "" {
		affs, err := extractFromOpenAlex(ctx, httpClient, p.Title, p.URL, "")
		if err == nil && len(affs) > 0 {
			slog.Debug("Extracted affiliations from OpenAlex", "paper", p.Title, "count", len(affs))
			return affs
		}
	}

	// 3. Fall back to LLM extraction if LLM client is available
	if llmClient != nil {
		affs, err := llmClient.ExtractAffiliations(ctx, p.Title, p.Authors, p.Abstract, p.FullText, modelName)
		if err != nil {
			slog.Warn("Failed to extract affiliations with LLM", "paper", p.Title, "error", err)
		} else if len(affs) > 0 {
			slog.Debug("Extracted affiliations via LLM", "paper", p.Title, "count", len(affs))
			return affs
		}
	}

	return nil
}

func extractFromURL(ctx context.Context, httpClient *http.Client, pageURL, pdfURL string) ([]string, error) {
	targetURL := pageURL
	if targetURL == "" {
		targetURL = pdfURL
	}
	if targetURL == "" {
		return nil, fmt.Errorf("empty URL")
	}

	// Normalize URL: if it's a PDF link, transform to webpage URL
	targetURL = strings.TrimSuffix(targetURL, ".full.pdf")
	targetURL = strings.TrimSuffix(targetURL, ".pdf")

	// If arXiv URL, check if HTML version is available
	if u, err := url.Parse(targetURL); err == nil {
		host := strings.ToLower(u.Hostname())
		if host == "arxiv.org" || host == "export.arxiv.org" {
			cleanPath := strings.TrimSuffix(u.Path, ".pdf")
			if m := arxivPathRegex.FindStringSubmatch(cleanPath); len(m) > 1 {
				arxivHTMLURL := fmt.Sprintf("https://arxiv.org/html/%s", m[1])
				affs, err := fetchAndParse(ctx, httpClient, arxivHTMLURL, parseArxivHTML)
				if err == nil && len(affs) > 0 {
					return affs, nil
				}
			}
		}
	}

	// For bioRxiv, medRxiv, and general publishers
	return fetchAndParse(ctx, httpClient, targetURL, parseMetaInstitutions)
}

func extractFromOpenAlex(ctx context.Context, httpClient *http.Client, title, paperURL, baseEndpoint string) ([]string, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	endpoint := "https://api.openalex.org/works"
	if baseEndpoint != "" {
		endpoint = baseEndpoint
	}

	var reqURL string
	if m := doiRegex.FindStringSubmatch(paperURL); len(m) > 1 {
		reqURL = fmt.Sprintf("%s?filter=doi:%s", endpoint, url.QueryEscape(m[1]))
	} else if title != "" {
		reqURL = fmt.Sprintf("%s?search=%s&per-page=1", endpoint, url.QueryEscape(title))
	} else {
		return nil, fmt.Errorf("no title or DOI available for OpenAlex lookup")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Zotero-arXiv-Daily/1.0 (mailto:digest@zotero-arxiv-daily.local)")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openalex status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var oaResp openAlexResponse
	if err := json.Unmarshal(bodyBytes, &oaResp); err != nil {
		return nil, err
	}

	if len(oaResp.Results) == 0 {
		return nil, nil
	}

	var raw []string
	for _, authorship := range oaResp.Results[0].Authorships {
		for _, inst := range authorship.Institutions {
			if strings.TrimSpace(inst.DisplayName) != "" {
				raw = append(raw, inst.DisplayName)
			}
		}
		if len(authorship.Institutions) == 0 {
			for _, affStr := range authorship.RawAffiliationStrings {
				if strings.TrimSpace(affStr) != "" {
					raw = append(raw, affStr)
				}
			}
		}
	}

	return cleanAndDeduplicate(raw), nil
}

func fetchAndParse(ctx context.Context, httpClient *http.Client, targetURL string, parser func(string) []string) ([]string, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code %d", resp.StatusCode)
	}

	// Read only the first 128KB, which encompasses the <head> section containing all meta tags
	headReader := io.LimitReader(resp.Body, 131072)
	bodyBytes, err := io.ReadAll(headReader)
	if err != nil {
		return nil, err
	}

	htmlContent := string(bodyBytes)
	affs := parser(htmlContent)
	return affs, nil
}

func parseMetaInstitutions(htmlContent string) []string {
	var raw []string
	tags := metaTagRegex.FindAllStringSubmatch(htmlContent, -1)
	for _, tag := range tags {
		attrs := tag[1]
		if metaNameRegex.MatchString(attrs) {
			if m := metaContentRegex.FindStringSubmatch(attrs); len(m) > 1 && strings.TrimSpace(m[1]) != "" {
				raw = append(raw, m[1])
			}
		}
	}
	return cleanAndDeduplicate(raw)
}

func parseArxivHTML(htmlContent string) []string {
	var raw []string
	matches := arxivAffiliationRegex.FindAllStringSubmatch(htmlContent, -1)
	for _, m := range matches {
		if len(m) > 1 && strings.TrimSpace(m[1]) != "" {
			raw = append(raw, m[1])
		}
	}
	return cleanAndDeduplicate(raw)
}

func cleanAndDeduplicate(raw []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, item := range raw {
		// Unescape HTML entities
		unescaped := html.UnescapeString(item)

		// Split on semicolons if an author has multiple institutions in one string
		parts := strings.Split(unescaped, ";")
		for _, part := range parts {
			cleaned := strings.TrimSpace(part)
			// Strip leading/trailing punctuation (e.g. trailing periods or commas)
			cleaned = strings.Trim(cleaned, " \t\r\n;,.")
			if cleaned == "" {
				continue
			}

			key := strings.ToLower(cleaned)
			if !seen[key] {
				seen[key] = true
				result = append(result, cleaned)
			}
		}
	}

	return result
}
