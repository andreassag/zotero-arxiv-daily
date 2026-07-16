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
