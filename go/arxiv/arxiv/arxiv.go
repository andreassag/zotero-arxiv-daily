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

package arxiv

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Paper represents a retrieved arXiv paper.
type Paper struct {
	Title    string
	Authors  []string
	Abstract string
	URL      string
	PDFURL   string
	EntryID  string
}

// Client is a client for fetching papers from arXiv.
type Client struct {
	httpClient *http.Client
	baseURL    string
	rssURL     string
	debug      bool
}

// Option is a functional option for configuring the Client.
type Option func(*Client)

// WithHTTPClient configures a custom http.Client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// WithBaseURL overrides the default arXiv API URL.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		c.baseURL = baseURL
	}
}

// WithRSSURL overrides the default arXiv RSS URL.
func WithRSSURL(rssURL string) Option {
	return func(c *Client) {
		c.rssURL = rssURL
	}
}

// WithDebug enables debug mode (e.g. limits output/fetching counts).
func WithDebug(debug bool) Option {
	return func(c *Client) {
		c.debug = debug
	}
}

// NewClient creates a new arXiv Client.
func NewClient(opts ...Option) *Client {
	c := &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    "https://export.arxiv.org/api/query",
		rssURL:     "https://rss.arxiv.org/atom/",
		debug:      false,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// xmlRSSEntry maps RSS feed entry.
type xmlRSSEntry struct {
	ID           string `xml:"id"`
	Title        string `xml:"title"`
	AnnounceType string `xml:"http://arxiv.org/schemas/atom announce_type"`
}

// xmlRSSFeed maps RSS feed root.
type xmlRSSFeed struct {
	XMLName xml.Name      `xml:"feed"`
	Entries []xmlRSSEntry `xml:"entry"`
}

// xmlAPIAuthor maps API author entry.
type xmlAPIAuthor struct {
	Name string `xml:"name"`
}

// xmlAPILink maps API link entry.
type xmlAPILink struct {
	Href  string `xml:"href,attr"`
	Rel   string `xml:"rel,attr"`
	Title string `xml:"title,attr"`
	Type  string `xml:"type,attr"`
}

// xmlAPIEntry maps API paper entry.
type xmlAPIEntry struct {
	ID      string         `xml:"id"`
	Title   string         `xml:"title"`
	Summary string         `xml:"summary"`
	Authors []xmlAPIAuthor `xml:"author"`
	Links   []xmlAPILink   `xml:"link"`
}

// xmlAPIFeed maps API feed root.
type xmlAPIFeed struct {
	XMLName xml.Name      `xml:"feed"`
	Entries []xmlAPIEntry `xml:"entry"`
}

// FetchPapers fetches new papers from arXiv RSS feed and then retrieves their details from the API.
func (c *Client) FetchPapers(ctx context.Context, categories []string, includeCrossList bool) ([]Paper, error) {
	if len(categories) == 0 {
		return nil, fmt.Errorf("at least one category must be specified")
	}

	// Fetch RSS feed
	query := strings.Join(categories, "+")
	rssEndpoint := fmt.Sprintf("%s%s", c.rssURL, query)

	req, err := http.NewRequestWithContext(ctx, "GET", rssEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create RSS request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute RSS request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RSS request failed with status: %d", resp.StatusCode)
	}

	var rssFeed xmlRSSFeed
	if err := xml.NewDecoder(resp.Body).Decode(&rssFeed); err != nil {
		return nil, fmt.Errorf("failed to decode RSS XML: %w", err)
	}

	allowedAnnounceTypes := map[string]bool{"new": true}
	if includeCrossList {
		allowedAnnounceTypes["cross"] = true
	}

	var paperIDs []string
	for _, entry := range rssFeed.Entries {
		annType := strings.TrimSpace(entry.AnnounceType)
		if annType == "" {
			annType = "new" // fallback
		}
		if allowedAnnounceTypes[annType] {
			id := entry.ID
			id = strings.TrimPrefix(id, "oai:arXiv.org:")
			if id != "" {
				paperIDs = append(paperIDs, id)
			}
		}
	}

	if c.debug && len(paperIDs) > 10 {
		paperIDs = paperIDs[:10]
	}

	if len(paperIDs) == 0 {
		return nil, nil
	}

	// Query arXiv API in batches of 20
	var allPapers []Paper
	batchSize := 20
	for i := 0; i < len(paperIDs); i += batchSize {
		end := i + batchSize
		if end > len(paperIDs) {
			end = len(paperIDs)
		}
		batch := paperIDs[i:end]

		batchPapers, err := c.fetchAPIDetails(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch details for batch %d: %w", i/batchSize, err)
		}
		allPapers = append(allPapers, batchPapers...)

		// Respect API etiquette
		if end < len(paperIDs) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(3 * time.Second):
			}
		}
	}

	return allPapers, nil
}

// fetchAPIDetails queries the arXiv API for a batch of paper IDs with exponential backoff on 429.
func (c *Client) fetchAPIDetails(ctx context.Context, ids []string) ([]Paper, error) {
	idList := strings.Join(ids, ",")
	apiURL := fmt.Sprintf("%s?id_list=%s", c.baseURL, url.QueryEscape(idList))

	var resp *http.Response

	maxRetries := 5
	backoff := 30 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create API request: %w", err)
		}

		resp, err = c.httpClient.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}

		if err == nil {
			resp.Body.Close()
		}

		// Handle 429 status code
		if (err == nil && resp.StatusCode == http.StatusTooManyRequests) || attempt < maxRetries-1 {
			wait := backoff * time.Duration(attempt+1)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
			continue
		}

		if err != nil {
			return nil, fmt.Errorf("API request failed: %w", err)
		}
		return nil, fmt.Errorf("API request failed with status: %d", resp.StatusCode)
	}
	defer resp.Body.Close()

	var apiFeed xmlAPIFeed
	if err := xml.NewDecoder(resp.Body).Decode(&apiFeed); err != nil {
		return nil, fmt.Errorf("failed to decode API XML: %w", err)
	}

	var papers []Paper
	for _, entry := range apiFeed.Entries {
		var authors []string
		for _, auth := range entry.Authors {
			authors = append(authors, auth.Name)
		}

		pdfURL := ""
		for _, link := range entry.Links {
			if link.Title == "pdf" || link.Type == "application/pdf" {
				pdfURL = link.Href
				break
			}
		}
		// Fallback: construct pdf url if not found in links
		if pdfURL == "" {
			parts := strings.Split(entry.ID, "/abs/")
			if len(parts) == 2 {
				pdfURL = fmt.Sprintf("https://arxiv.org/pdf/%s", parts[1])
			}
		}

		// Format Title & Abstract by cleaning up extra whitespace
		title := cleanText(entry.Title)
		abstract := cleanText(entry.Summary)

		papers = append(papers, Paper{
			Title:    title,
			Authors:  authors,
			Abstract: abstract,
			URL:      entry.ID,
			PDFURL:   pdfURL,
			EntryID:  entry.ID,
		})
	}

	return papers, nil
}

// cleanText normalizes spaces and removes newlines from Title or Abstract
func cleanText(text string) string {
	lines := strings.Split(text, "\n")
	var cleaned []string
	for _, line := range lines {
		cleaned = append(cleaned, strings.TrimSpace(line))
	}
	return strings.Join(cleaned, " ")
}

// GetRawRSSBody fetches the raw XML from arXiv RSS for testing/debugging.
func (c *Client) GetRawRSSBody(ctx context.Context, category string) (string, error) {
	rssEndpoint := fmt.Sprintf("%s%s", c.rssURL, category)
	req, err := http.NewRequestWithContext(ctx, "GET", rssEndpoint, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(bodyBytes), nil
}
