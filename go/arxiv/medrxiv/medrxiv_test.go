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

package medrxiv

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const mockMedrxivJSON = `{
  "messages": [{"status": "ok"}],
  "collection": [
    {
      "doi": "10.1101/2026.03.01.000003",
      "title": "Medrxiv Title 1",
      "authors": "Johnson, B.",
      "abstract": "Abstract Medical",
      "date": "2026-03-03",
      "category": "Neurology",
      "version": "1"
    }
  ]
}`

func TestMedrxivFetchPapers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Confirm path contains "medrxiv" server target.
		if !strings.HasPrefix(r.URL.Path, "/medrxiv") {
			t.Errorf("expected path to start with /medrxiv, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockMedrxivJSON))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))

	papers, err := client.FetchPapers(context.Background(), []string{"Neurology"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(papers) != 1 {
		t.Fatalf("expected 1 paper, got %d", len(papers))
	}

	p := papers[0]
	if p.Title != "Medrxiv Title 1" {
		t.Errorf("expected 'Medrxiv Title 1', got '%s'", p.Title)
	}

	wantPDF := "https://www.medrxiv.org/content/10.1101/2026.03.01.000003v1.full.pdf"
	if p.PDFURL != wantPDF {
		t.Errorf("expected medrxiv PDF URL %q, got %q", wantPDF, p.PDFURL)
	}
}

const mockMedrxivRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns="http://purl.org/rss/1.0/" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <item rdf:about="https://www.medrxiv.org/content/10.1101/2026.09.25.000003v1?rss=1">
    <title>Medrxiv RSS Title</title>
    <link>https://www.medrxiv.org/content/10.1101/2026.09.25.000003v1?rss=1</link>
    <description>Medrxiv RSS Abstract.</description>
    <dc:creator>Johnson, B.</dc:creator>
    <dc:date>2026-09-26</dc:date>
    <dc:identifier>doi:10.1101/2026.09.25.000003</dc:identifier>
  </item>
</rdf:RDF>`

func TestMedrxivFetchPapersRSSFallback(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Empty body (0 bytes)
	}))
	defer apiServer.Close()

	rssServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockMedrxivRSS))
	}))
	defer rssServer.Close()

	client := NewClient(
		WithBaseURL(apiServer.URL),
		WithRSSBaseURL(rssServer.URL),
	)

	papers, err := client.FetchPapers(context.Background(), []string{"Neurology"})
	if err != nil {
		t.Fatalf("expected no error with RSS fallback, got: %v", err)
	}

	if len(papers) != 1 {
		t.Fatalf("expected 1 paper from RSS, got %d", len(papers))
	}

	p := papers[0]
	if p.Title != "Medrxiv RSS Title" {
		t.Errorf("expected 'Medrxiv RSS Title', got %q", p.Title)
	}
	if p.Abstract != "Medrxiv RSS Abstract." {
		t.Errorf("expected RSS abstract, got %q", p.Abstract)
	}
	if p.URL != "https://www.medrxiv.org/content/10.1101/2026.09.25.000003v1" {
		t.Errorf("expected clean URL, got %q", p.URL)
	}
	if p.PDFURL != "https://www.medrxiv.org/content/10.1101/2026.09.25.000003v1.full.pdf" {
		t.Errorf("expected full.pdf URL, got %q", p.PDFURL)
	}
}

func TestMedrxivFetchPapersEmptyCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"messages":[{"status":"ok"}],"collection":[]}`))
	}))
	defer server.Close()

	rssServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockMedrxivRSS))
	}))
	defer rssServer.Close()

	client := NewClient(WithBaseURL(server.URL), WithRSSBaseURL(rssServer.URL))

	papers, err := client.FetchPapers(context.Background(), []string{"Neurology"})
	if err != nil {
		t.Fatalf("expected no error with RSS fallback, got: %v", err)
	}
	if len(papers) != 1 {
		t.Fatalf("expected 1 paper from RSS fallback, got %d", len(papers))
	}
}
