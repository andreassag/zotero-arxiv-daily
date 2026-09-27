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

package biorxiv

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

// NewClient creates a new Client (defaults to bioRxiv).
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
		return nil, fmt.Errorf("REST API failed (%v) and RSS fallback failed: %w", err, rssErr)
	}
	return rssPapers, nil
}

func (c *Client) fetchFromAPI(ctx context.Context, categories []string) ([]Paper, error) {
	apiURL := fmt.Sprintf("%s/%s/2d", c.baseURL, c.server)

	var (
		resp  *http.Response
		doErr error
	)

	maxRetries := 3
	delay := 2 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		resp, doErr = c.httpClient.Do(req)
		if doErr == nil && resp.StatusCode == http.StatusOK {
			break
		}

		if doErr == nil {
			resp.Body.Close()
		}

		if attempt < maxRetries-1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			continue
		}

		if doErr != nil {
			return nil, fmt.Errorf("failed to call API: %w", doErr)
		}
		return nil, fmt.Errorf("API call failed with status: %d", resp.StatusCode)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if len(strings.TrimSpace(string(bodyBytes))) == 0 {
		return nil, fmt.Errorf("API returned empty response (0 bytes)")
	}

	var result apiResponse
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to decode JSON response: %w", err)
	}

	if len(result.Collection) == 0 {
		return nil, nil
	}

	// Find the latest date in the collection.
	dates := make(map[string]bool)
	for _, item := range result.Collection {
		if item.Date != "" {
			dates[item.Date] = true
		}
	}
	if len(dates) == 0 {
		return nil, nil
	}

	sortedDates := make([]string, 0, len(dates))
	for date := range dates {
		sortedDates = append(sortedDates, date)
	}
	sort.Strings(sortedDates)
	latestDate := sortedDates[len(sortedDates)-1]

	// Case-insensitive category filter.
	allowedCategories := make(map[string]bool)
	for _, cat := range categories {
		allowedCategories[strings.ToLower(cat)] = true
	}

	var papers []Paper
	for _, item := range result.Collection {
		if item.Date != latestDate {
			continue
		}
		if !allowedCategories[strings.ToLower(item.Category)] {
			continue
		}

		authorParts := strings.Split(item.Authors, ";")
		authors := make([]string, 0, len(authorParts))
		for _, auth := range authorParts {
			if cleaned := strings.TrimSpace(auth); cleaned != "" {
				authors = append(authors, cleaned)
			}
		}

		pageURL := fmt.Sprintf("https://www.%s.org/content/%sv%s", c.server, item.DOI, item.Version)
		pdfURL := fmt.Sprintf("https://www.%s.org/content/%sv%s.full.pdf", c.server, item.DOI, item.Version)

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

	type collectedItem struct {
		item     rssItem
		category string
	}

	var allItems []collectedItem
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

		bodyBytes, err := io.ReadAll(resp.Body)
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
			allItems = append(allItems, collectedItem{
				item:     itm,
				category: cat,
			})
		}
	}

	if len(allItems) == 0 {
		return nil, nil
	}

	// Group and find dates
	dates := make(map[string]int)
	for _, itm := range allItems {
		d := strings.TrimSpace(itm.item.Date)
		if d != "" {
			dates[d]++
		}
	}

	if len(dates) == 0 {
		return nil, nil
	}

	sortedDates := make([]string, 0, len(dates))
	for d := range dates {
		sortedDates = append(sortedDates, d)
	}
	sort.Strings(sortedDates)

	// Filter by latest date(s)
	// If the latest date has < 5 papers and a previous date exists, include both dates.
	latestDate := sortedDates[len(sortedDates)-1]
	allowedDates := map[string]bool{latestDate: true}
	if dates[latestDate] < 5 && len(sortedDates) > 1 {
		prevDate := sortedDates[len(sortedDates)-2]
		allowedDates[prevDate] = true
	}

	var papers []Paper
	for _, ci := range allItems {
		d := strings.TrimSpace(ci.item.Date)
		if !allowedDates[d] {
			continue
		}

		title := cleanText(ci.item.Title)
		abstract := cleanText(ci.item.Description)
		authors := parseAuthors(ci.item.Creator)

		cleanLink := strings.TrimSpace(ci.item.Link)
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

func cleanText(text string) string {
	lines := strings.Split(text, "\n")
	var cleaned []string
	for _, line := range lines {
		if s := strings.TrimSpace(line); s != "" {
			cleaned = append(cleaned, s)
		}
	}
	return strings.Join(cleaned, " ")
}

func parseAuthors(creator string) []string {
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
