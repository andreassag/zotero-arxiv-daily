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
	"net/http"
	"net/http/httptest"
	"testing"
)

const mockBiorxivJSON = `{
  "messages": [{"status": "ok"}],
  "collection": [
    {
      "doi": "10.1101/2026.03.01.000001",
      "title": "Biorxiv Title 1",
      "authors": "Smith, J.; Doe, A.",
      "abstract": "Abstract 1",
      "date": "2026-03-02",
      "category": "Bioinformatics",
      "version": "1"
    },
    {
      "doi": "10.1101/2026.03.01.000002",
      "title": "Biorxiv Title 2",
      "authors": "Wang, L.",
      "abstract": "Abstract 2",
      "date": "2026-03-01",
      "category": "Bioinformatics",
      "version": "1"
    }
  ]
}`

func TestFetchPapers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockBiorxivJSON))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))

	papers, err := client.FetchPapers(context.Background(), []string{"Bioinformatics"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Only the latest-date entry (2026-03-02) with the matching category should be returned.
	if len(papers) != 1 {
		t.Fatalf("expected 1 paper, got %d", len(papers))
	}

	p := papers[0]
	if p.Title != "Biorxiv Title 1" {
		t.Errorf("expected 'Biorxiv Title 1', got '%s'", p.Title)
	}
	if len(p.Authors) != 2 || p.Authors[0] != "Smith, J." || p.Authors[1] != "Doe, A." {
		t.Errorf("expected 2 authors, got %v", p.Authors)
	}
	if p.Abstract != "Abstract 1" {
		t.Errorf("expected 'Abstract 1', got '%s'", p.Abstract)
	}

	wantPDF := "https://www.biorxiv.org/content/10.1101/2026.03.01.000001v1.full.pdf"
	if p.PDFURL != wantPDF {
		t.Errorf("expected PDF URL %q, got %q", wantPDF, p.PDFURL)
	}
}

func TestFetchPapersEmptyCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"messages":[{"status":"ok"}],"collection":[]}`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))

	papers, err := client.FetchPapers(context.Background(), []string{"Bioinformatics"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if papers != nil {
		t.Errorf("expected nil papers for empty collection, got %v", papers)
	}
}

func TestFetchPapersCategoryFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockBiorxivJSON))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))

	// Request a category not present in the mock → should return no papers.
	papers, err := client.FetchPapers(context.Background(), []string{"Neuroscience"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(papers) != 0 {
		t.Errorf("expected 0 papers for non-matching category, got %d", len(papers))
	}
}

const mockBiorxivRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns="http://purl.org/rss/1.0/" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <item rdf:about="https://www.biorxiv.org/content/10.1101/2026.09.25.000001v1?rss=1">
    <title>RSS Fallback Paper Title</title>
    <link>https://www.biorxiv.org/content/10.1101/2026.09.25.000001v1?rss=1</link>
    <description>This is the abstract from RSS.</description>
    <dc:creator>Smith, J., Doe, A.</dc:creator>
    <dc:date>2026-09-26</dc:date>
    <dc:identifier>doi:10.1101/2026.09.25.000001</dc:identifier>
  </item>
</rdf:RDF>`

func TestFetchPapersRSSFallback(t *testing.T) {
	// REST API returns empty 200 (like bioRxiv during outage)
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Empty body (0 bytes)
	}))
	defer apiServer.Close()

	rssServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockBiorxivRSS))
	}))
	defer rssServer.Close()

	client := NewClient(
		WithBaseURL(apiServer.URL),
		WithRSSBaseURL(rssServer.URL),
	)

	papers, err := client.FetchPapers(context.Background(), []string{"Bioinformatics"})
	if err != nil {
		t.Fatalf("expected no error with RSS fallback, got: %v", err)
	}

	if len(papers) != 1 {
		t.Fatalf("expected 1 paper from RSS, got %d", len(papers))
	}

	p := papers[0]
	if p.Title != "RSS Fallback Paper Title" {
		t.Errorf("expected 'RSS Fallback Paper Title', got %q", p.Title)
	}
	if p.Abstract != "This is the abstract from RSS." {
		t.Errorf("expected RSS abstract, got %q", p.Abstract)
	}
	if p.URL != "https://www.biorxiv.org/content/10.1101/2026.09.25.000001v1" {
		t.Errorf("expected clean URL without rss param, got %q", p.URL)
	}
	if p.PDFURL != "https://www.biorxiv.org/content/10.1101/2026.09.25.000001v1.full.pdf" {
		t.Errorf("expected full.pdf URL, got %q", p.PDFURL)
	}
}
