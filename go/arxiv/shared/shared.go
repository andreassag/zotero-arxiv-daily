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

package shared

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const maxResponseBytes = 10 << 20 // 10 MB limit for HTTP responses

// Paper represents a bioRxiv or medRxiv paper.
type Paper struct {
	Title    string
	Authors  []string
	Abstract string
	URL      string
	PDFURL   string
}

// Client fetches papers from the bioRxiv or medRxiv API (with RSS fallback).
type Client struct {
	httpClient *http.Client
	server     string // "biorxiv" or "medrxiv"
	baseURL    string
	rssBaseURL string
	debug      bool
}

// Option is a functional option for configuring a Client.
type Option func(*Client)

// WithHTTPClient configures a custom http.Client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// WithBaseURL overrides the default API URL prefix.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		c.baseURL = baseURL
	}
}

// WithRSSBaseURL overrides the default RSS feed URL prefix.
func WithRSSBaseURL(rssBaseURL string) Option {
	return func(c *Client) {
		c.rssBaseURL = rssBaseURL
	}
}

// WithDebug enables debug mode.
func WithDebug(debug bool) Option {
	return func(c *Client) {
		c.debug = debug
	}
}

// WithServer configures the server target ("biorxiv" or "medrxiv").
func WithServer(server string) Option {
	return func(c *Client) {
		c.server = server
	}
}

// NewClient creates a new Client with sensible defaults.
func NewClient(opts ...Option) *Client {
	c := &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		server:     "biorxiv",
		baseURL:    "https://api.biorxiv.org/details",
		rssBaseURL: "https://connect.biorxiv.org/biorxiv_xml.php",
		debug:      false,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// responseMessage is the JSON messages field.
type responseMessage struct {
	Status string `json:"status"`
}

// responseCollection is one item in the JSON collection.
type responseCollection struct {
	DOI      string `json:"doi"`
	Title    string `json:"title"`
	Authors  string `json:"authors"`
	Abstract string `json:"abstract"`
	Date     string `json:"date"`
	Category string `json:"category"`
	Version  string `json:"version"`
}

// apiResponse is the root JSON structure.
type apiResponse struct {
	Messages   []responseMessage    `json:"messages"`
	Collection []responseCollection `json:"collection"`
}

// rssItem represents an item in the RSS/RDF feed.
type rssItem struct {
	About       string `xml:"about,attr"`
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Creator     string `xml:"creator"`
	Date        string `xml:"date"`
	Identifier  string `xml:"identifier"`
}

// rssFeed represents the root RDF element in the RSS feed.
type rssFeed struct {
	XMLName xml.Name  `xml:"RDF"`
	Items   []rssItem `xml:"item"`
}

// FetchPapers fetches new papers from the bioRxiv or medRxiv API, falling back to RSS feeds if the API is down or empty.
func (c *Client) FetchPapers(ctx context.Context, categories []string) ([]Paper, error) {
	papers, err := c.fetchFromAPI(ctx, categories)
	if err == nil {
		return papers, nil
	}

	slog.Warn("Failed to fetch papers from REST API, falling back to RSS feed", "server", c.server, "error", err)

	rssPapers, rssErr := c.fetchFromRSS(ctx, categories)
	if rssErr != nil {
		return nil, fmt.Errorf("both API and RSS fallback failed: api error: %w; rss error: %v", err, rssErr)
	}
	return rssPapers, nil
}

func (c *Client) fetchFromAPI(ctx context.Context, categories []string) ([]Paper, error) {
	if len(categories) == 0 {
		return nil, nil
	}

	catMap := make(map[string]bool)
	for _, cat := range categories {
		catMap[strings.ToLower(strings.TrimSpace(cat))] = true
	}

	// Fetch recent papers for the server
	apiEndpoint := fmt.Sprintf("%s/%s", strings.TrimRight(c.baseURL, "/"), c.server)
	req, err := http.NewRequestWithContext(ctx, "GET", apiEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create API request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned non-200 status code: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to read API response body: %w", err)
	}

	if len(bodyBytes) == 0 {
		return nil, fmt.Errorf("API returned empty body")
	}

	var apiResp apiResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse API response JSON: %w", err)
	}

	if len(apiResp.Messages) > 0 && apiResp.Messages[0].Status != "ok" {
		return nil, fmt.Errorf("API returned status message: %s", apiResp.Messages[0].Status)
	}

	if len(apiResp.Collection) == 0 {
		return nil, fmt.Errorf("API returned empty collection")
	}

	// Group and find dates using parsed chronological comparison.
	dates := make(map[time.Time]int)
	for _, item := range apiResp.Collection {
		if t, ok := ParseDate(item.Date); ok {
			dates[t]++
		}
	}

	if len(dates) == 0 {
		return nil, fmt.Errorf("API returned collection with no valid dates")
	}

	sortedDates := make([]time.Time, 0, len(dates))
	for d := range dates {
		sortedDates = append(sortedDates, d)
	}
	sort.Slice(sortedDates, func(i, j int) bool {
		return sortedDates[i].Before(sortedDates[j])
	})

	latestDate := sortedDates[len(sortedDates)-1]
	allowedDates := map[time.Time]bool{latestDate: true}
	if dates[latestDate] < 5 && len(sortedDates) > 1 {
		prevDate := sortedDates[len(sortedDates)-2]
		allowedDates[prevDate] = true
	}

	var papers []Paper
	for _, item := range apiResp.Collection {
		t, ok := ParseDate(item.Date)
		if !ok || !allowedDates[t] {
			continue
		}

		if !catMap[strings.ToLower(strings.TrimSpace(item.Category))] {
			continue
		}

		authorParts := strings.Split(item.Authors, ";")
		authors := make([]string, 0, len(authorParts))
		for _, auth := range authorParts {
			if cleaned := strings.TrimSpace(auth); cleaned != "" {
				authors = append(authors, cleaned)
			}
		}

		pageURL := fmt.Sprintf("https://www.%s.org/content/%s", c.server, item.DOI)
		pdfURL := fmt.Sprintf("https://www.%s.org/content/%s.full.pdf", c.server, item.DOI)
		if item.Version != "" {
			pageURL = fmt.Sprintf("https://www.%s.org/content/%sv%s", c.server, item.DOI, item.Version)
			pdfURL = fmt.Sprintf("https://www.%s.org/content/%sv%s.full.pdf", c.server, item.DOI, item.Version)
		}

		papers = append(papers, Paper{
			Title:    item.Title,
			Authors:  authors,
			Abstract: item.Abstract,
			URL:      pageURL,
			PDFURL:   pdfURL,
		})
	}

	if c.debug && len(papers) > 10 {
		papers = papers[:10]
	}

	return papers, nil
}

func (c *Client) fetchFromRSS(ctx context.Context, categories []string) ([]Paper, error) {
	if len(categories) == 0 {
		return nil, nil
	}

	var allItems []rssItem
	seenKeys := make(map[string]bool)

	for _, cat := range categories {
		subject := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(cat), " ", "_"))
		reqURL := c.rssBaseURL
		if strings.Contains(reqURL, "?") {
			reqURL = fmt.Sprintf("%s&subject=%s", reqURL, url.QueryEscape(subject))
		} else {
			reqURL = fmt.Sprintf("%s?subject=%s", reqURL, url.QueryEscape(subject))
		}

		req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create RSS request for %s: %w", cat, err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			slog.Warn("Failed to fetch RSS feed for category", "category", cat, "error", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			slog.Warn("RSS feed returned non-200 status", "category", cat, "status", resp.StatusCode)
			continue
		}

		bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		resp.Body.Close()
		if err != nil {
			slog.Warn("Failed to read RSS feed body", "category", cat, "error", err)
			continue
		}

		var feed rssFeed
		if err := xml.Unmarshal(bodyBytes, &feed); err != nil {
			slog.Warn("Failed to parse RSS feed XML", "category", cat, "error", err)
			continue
		}

		for _, itm := range feed.Items {
			cleanLink := strings.TrimSpace(itm.Link)
			if idx := strings.Index(cleanLink, "?"); idx != -1 {
				cleanLink = cleanLink[:idx]
			}
			key := cleanLink
			if key == "" {
				key = strings.TrimSpace(itm.Identifier)
			}
			if key == "" {
				key = strings.TrimSpace(itm.Title)
			}
			if key == "" || seenKeys[key] {
				continue
			}
			seenKeys[key] = true
			allItems = append(allItems, itm)
		}
	}

	if len(allItems) == 0 {
		return nil, nil
	}

	dates := make(map[time.Time]int)
	for _, itm := range allItems {
		if t, ok := ParseDate(itm.Date); ok {
			dates[t]++
		}
	}

	if len(dates) == 0 {
		return nil, nil
	}

	sortedDates := make([]time.Time, 0, len(dates))
	for d := range dates {
		sortedDates = append(sortedDates, d)
	}
	sort.Slice(sortedDates, func(i, j int) bool {
		return sortedDates[i].Before(sortedDates[j])
	})

	latestDate := sortedDates[len(sortedDates)-1]
	allowedDates := map[time.Time]bool{latestDate: true}
	if dates[latestDate] < 5 && len(sortedDates) > 1 {
		prevDate := sortedDates[len(sortedDates)-2]
		allowedDates[prevDate] = true
	}

	var papers []Paper
	for _, itm := range allItems {
		t, ok := ParseDate(itm.Date)
		if !ok || !allowedDates[t] {
			continue
		}

		title := CleanText(itm.Title)
		abstract := CleanText(itm.Description)
		authors := ParseAuthors(itm.Creator)

		cleanLink := strings.TrimSpace(itm.Link)
		if idx := strings.Index(cleanLink, "?"); idx != -1 {
			cleanLink = cleanLink[:idx]
		}

		pdfURL := cleanLink
		if !strings.HasSuffix(pdfURL, ".full.pdf") {
			pdfURL = cleanLink + ".full.pdf"
		}

		papers = append(papers, Paper{
			Title:    title,
			Authors:  authors,
			Abstract: abstract,
			URL:      cleanLink,
			PDFURL:   pdfURL,
		})
	}

	if c.debug && len(papers) > 10 {
		papers = papers[:10]
	}

	return papers, nil
}

// CleanText normalizes spaces and removes newlines from Title or Abstract.
func CleanText(text string) string {
	lines := strings.Split(text, "\n")
	var cleaned []string
	for _, line := range lines {
		if s := strings.TrimSpace(line); s != "" {
			cleaned = append(cleaned, s)
		}
	}
	return strings.Join(cleaned, " ")
}

// ParseAuthors parses author names separated by semicolons or commas.
func ParseAuthors(creator string) []string {
	creator = strings.TrimSpace(creator)
	if creator == "" {
		return nil
	}
	if strings.Contains(creator, ";") {
		parts := strings.Split(creator, ";")
		var authors []string
		for _, p := range parts {
			if s := strings.TrimSpace(p); s != "" {
				authors = append(authors, s)
			}
		}
		return authors
	}
	parts := strings.Split(creator, ",")
	var authors []string
	for i := 0; i < len(parts); i++ {
		p := strings.TrimSpace(parts[i])
		if p == "" {
			continue
		}
		if i+1 < len(parts) {
			next := strings.TrimSpace(parts[i+1])
			if !strings.Contains(next, " ") || len(next) <= 3 || strings.HasSuffix(next, ".") {
				authors = append(authors, p+", "+next)
				i++
				continue
			}
		}
		authors = append(authors, p)
	}
	return authors
}

// ParseDate parses a date string supporting multiple standard and non-standard formats (e.g. "YYYY-MM-DD", "YYYY-M-D").
func ParseDate(d string) (time.Time, bool) {
	d = strings.TrimSpace(d)
	if d == "" {
		return time.Time{}, false
	}
	layouts := []string{
		"2006-01-02",
		"2006-1-2",
		"2006-01-2",
		"2006-1-02",
		time.RFC3339,
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, d); err == nil {
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), true
		}
	}
	return time.Time{}, false
}
