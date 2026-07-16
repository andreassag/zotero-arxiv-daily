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

package zotero

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// CorpusPaper represents a paper fetched from Zotero.
type CorpusPaper struct {
	Title     string    `json:"title"`
	Abstract  string    `json:"abstract"`
	AddedDate time.Time `json:"added_date"`
	Paths     []string  `json:"paths"`
}

// Client is a client for the Zotero API.
type Client struct {
	httpClient *http.Client
	userID     string
	apiKey     string
	baseURL    string
}

// Option is a functional option for configuring the Client.
type Option func(*Client)

// WithHTTPClient configures a custom http.Client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// WithBaseURL overrides the default Zotero API URL prefix.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		c.baseURL = baseURL
	}
}

// NewClient creates a new Zotero Client.
func NewClient(userID, apiKey string, opts ...Option) *Client {
	c := &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		userID:     userID,
		apiKey:     apiKey,
		baseURL:    "https://api.zotero.org",
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// xml/json API mappings for collections
type collectionData struct {
	Name             string      `json:"name"`
	ParentCollection interface{} `json:"parentCollection"` // string or bool (false)
}

type collectionResponse struct {
	Key  string         `json:"key"`
	Data collectionData `json:"data"`
}

// xml/json API mappings for items
type itemData struct {
	Title        string   `json:"title"`
	AbstractNote string   `json:"abstractNote"`
	DateAdded    string   `json:"dateAdded"`
	Collections  []string `json:"collections"`
}

type itemResponse struct {
	Key  string   `json:"key"`
	Data itemData `json:"data"`
}

// FetchCorpus retrieves conference papers, journal articles, and preprints from the user's Zotero database,
// constructs collection paths, and filters out papers with empty abstracts.
func (c *Client) FetchCorpus(ctx context.Context) ([]CorpusPaper, error) {
	// 1. Fetch all collections
	collections, err := c.fetchAllCollections(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch collections: %w", err)
	}

	// Index collections by key for O(1) lookup
	colMap := make(map[string]collectionResponse)
	for _, col := range collections {
		colMap[col.Key] = col
	}

	// Define recursive path resolution function
	var getCollectionPath func(string) string
	getCollectionPath = func(key string) string {
		col, ok := colMap[key]
		if !ok {
			return ""
		}
		// parentCollection can be a string key, or boolean false
		parentKey := ""
		if parentStr, ok := col.Data.ParentCollection.(string); ok {
			parentKey = parentStr
		}

		if parentKey != "" {
			parentPath := getCollectionPath(parentKey)
			if parentPath != "" {
				return parentPath + "/" + col.Data.Name
			}
		}
		return col.Data.Name
	}

	// 2. Fetch all items
	items, err := c.fetchAllItems(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch items: %w", err)
	}

	var corpus []CorpusPaper
	for _, item := range items {
		// Filter papers without abstract Note (matches Python executor logic)
		if item.Data.AbstractNote == "" {
			continue
		}

		// Calculate collection paths
		var paths []string
		for _, colKey := range item.Data.Collections {
			path := getCollectionPath(colKey)
			if path != "" {
				paths = append(paths, path)
			}
		}

		// Parse date added
		addedDate, err := time.Parse(time.RFC3339, item.Data.DateAdded)
		if err != nil {
			// Fallback: if time parsing fails, use current time
			addedDate = time.Now()
		}

		corpus = append(corpus, CorpusPaper{
			Title:     item.Data.Title,
			Abstract:  item.Data.AbstractNote,
			AddedDate: addedDate,
			Paths:     paths,
		})
	}

	return corpus, nil
}

func (c *Client) fetchAllCollections(ctx context.Context) ([]collectionResponse, error) {
	var allCollections []collectionResponse
	start := 0
	limit := 100

	for {
		apiURL := fmt.Sprintf("%s/users/%s/collections?start=%d&limit=%d", c.baseURL, c.userID, start, limit)
		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Zotero-API-Key", c.apiKey)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("HTTP status %d fetching collections", resp.StatusCode)
		}

		var page []collectionResponse
		err = json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		if len(page) == 0 {
			break
		}

		allCollections = append(allCollections, page...)
		if len(page) < limit {
			break
		}
		start += len(page)
	}

	return allCollections, nil
}

func (c *Client) fetchAllItems(ctx context.Context) ([]itemResponse, error) {
	var allItems []itemResponse
	start := 0
	limit := 100

	// Item types we want: conferencePaper || journalArticle || preprint
	itemTypes := "conferencePaper || journalArticle || preprint"

	for {
		baseURL, err := url.Parse(c.baseURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse base URL: %w", err)
		}
		baseURL.Path = fmt.Sprintf("/users/%s/items", c.userID)
		query := baseURL.Query()
		query.Set("itemType", itemTypes)
		query.Set("start", strconv.Itoa(start))
		query.Set("limit", strconv.Itoa(limit))
		baseURL.RawQuery = query.Encode()
		apiURL := baseURL.String()
		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Zotero-API-Key", c.apiKey)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		// Zotero supports Total-Results header which we can check, but standard limit check is fine.
		totalResultsHeader := resp.Header.Get("Total-Results")
		totalResults := -1
		if totalResultsHeader != "" {
			if tr, err := strconv.Atoi(totalResultsHeader); err == nil {
				totalResults = tr
			}
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("HTTP status %d fetching items", resp.StatusCode)
		}

		var page []itemResponse
		err = json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		if len(page) == 0 {
			break
		}

		allItems = append(allItems, page...)
		if totalResults != -1 && len(allItems) >= totalResults {
			break
		}
		if len(page) < limit {
			break
		}
		start += len(page)
	}

	return allItems, nil
}
