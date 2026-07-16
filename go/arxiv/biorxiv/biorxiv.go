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
	"fmt"
	"net/http"
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

// Client fetches papers from the bioRxiv or medRxiv API.
type Client struct {
	httpClient *http.Client
	server     string // "biorxiv" or "medrxiv"
	baseURL    string
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

// FetchPapers fetches new papers from the bioRxiv or medRxiv API for the given categories.
func (c *Client) FetchPapers(ctx context.Context, categories []string) ([]Paper, error) {
	apiURL := fmt.Sprintf("%s/%s/2d", c.baseURL, c.server)

	var (
		resp  *http.Response
		doErr error
	)

	maxRetries := 10
	delay := 10 * time.Second

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

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
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

		pdfURL := fmt.Sprintf("https://www.%s.org/content/%sv%s.full.pdf", c.server, item.DOI, item.Version)

		papers = append(papers, Paper{
			Title:    item.Title,
			Authors:  authors,
			Abstract: item.Abstract,
			URL:      pdfURL,
			PDFURL:   pdfURL,
		})
	}

	return papers, nil
}
