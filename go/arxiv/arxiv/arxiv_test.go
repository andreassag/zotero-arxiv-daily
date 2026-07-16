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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// arXiv tests
// ---------------------------------------------------------------------------

const mockRSS = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns:arxiv="http://arxiv.org/schemas/atom" xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>oai:arXiv.org:2508.13426v1</id>
    <title>Test Title 1</title>
    <arxiv:announce_type>new</arxiv:announce_type>
  </entry>
  <entry>
    <id>oai:arXiv.org:2508.13427v1</id>
    <title>Test Title 2</title>
    <arxiv:announce_type>cross</arxiv:announce_type>
  </entry>
</feed>`

const mockAPI = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>http://arxiv.org/abs/2508.13426v1</id>
    <title>Test Title 1</title>
    <summary>Abstract 1</summary>
    <author><name>Author A</name></author>
    <link title="pdf" href="http://arxiv.org/pdf/2508.13426v1" rel="related" type="application/pdf"/>
  </entry>
  <entry>
    <id>http://arxiv.org/abs/2508.13427v1</id>
    <title>Test Title 2</title>
    <summary>Abstract 2</summary>
    <author><name>Author B</name></author>
    <link title="pdf" href="http://arxiv.org/pdf/2508.13427v1" rel="related" type="application/pdf"/>
  </entry>
</feed>`

func TestArxivFetchPapers(t *testing.T) {
	rssServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockRSS))
	}))
	defer rssServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		w.WriteHeader(http.StatusOK)

		// Parse id_list query parameter and return only requested papers
		query := r.URL.Query()
		idList := query.Get("id_list")

		// If no specific IDs requested, return all (for backward compatibility)
		if idList == "" {
			_, _ = w.Write([]byte(mockAPI))
			return
		}

		// Build response based on requested IDs
		requestedIDs := strings.Split(idList, ",")
		requestedSet := make(map[string]bool)
		for _, id := range requestedIDs {
			requestedSet[strings.TrimSpace(id)] = true
		}

		// Parse mockAPI and filter
		var apiFeed xmlAPIFeed
		if err := xml.Unmarshal([]byte(mockAPI), &apiFeed); err != nil {
			http.Error(w, "failed to parse mock API", http.StatusInternalServerError)
			return
		}

		var filteredEntries []xmlAPIEntry
		for _, entry := range apiFeed.Entries {
			// Extract ID from entry.ID (format: http://arxiv.org/abs/2508.13426v1)
			parts := strings.Split(entry.ID, "/abs/")
			var entryID string
			if len(parts) == 2 {
				entryID = parts[1]
			} else {
				entryID = entry.ID
			}
			// Also handle oai:arXiv.org: prefix if present
			entryID = strings.TrimPrefix(entryID, "oai:arXiv.org:")

			if requestedSet[entryID] {
				filteredEntries = append(filteredEntries, entry)
			}
		}

		// Reconstruct XML with only filtered entries
		result := xmlAPIFeed{Entries: filteredEntries}
		xmlBytes, err := xml.Marshal(result)
		if err != nil {
			http.Error(w, "failed to marshal XML", http.StatusInternalServerError)
			return
		}

		_, _ = w.Write(xmlBytes)
	}))
	defer apiServer.Close()

	client := NewClient(
		WithRSSURL(rssServer.URL+"/"),
		WithBaseURL(apiServer.URL),
	)

	// Without cross-list papers.
	papers, err := client.FetchPapers(context.Background(), []string{"cs.AI"}, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(papers) != 1 {
		t.Errorf("expected 1 paper, got %d", len(papers))
	} else {
		p := papers[0]
		if p.Title != "Test Title 1" {
			t.Errorf("expected 'Test Title 1', got '%s'", p.Title)
		}
		if len(p.Authors) != 1 || p.Authors[0] != "Author A" {
			t.Errorf("expected Author A, got %v", p.Authors)
		}
		if p.Abstract != "Abstract 1" {
			t.Errorf("expected Abstract 1, got '%s'", p.Abstract)
		}
		if p.PDFURL != "http://arxiv.org/pdf/2508.13426v1" {
			t.Errorf("expected PDF URL, got '%s'", p.PDFURL)
		}
	}

	// With cross-list papers.
	papersWithCross, err := client.FetchPapers(context.Background(), []string{"cs.AI"}, true)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(papersWithCross) != 2 {
		t.Errorf("expected 2 papers with cross list, got %d", len(papersWithCross))
	}
}
