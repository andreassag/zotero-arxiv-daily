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
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestCleanText(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"   hello   world   ", "hello   world"},
		{"line 1\nline 2\n\nline 3\n", "line 1 line 2 line 3"},
		{"\n\n\n", ""},
	}
	for _, tt := range tests {
		got := CleanText(tt.input)
		if got != tt.expected {
			t.Errorf("CleanText(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestParseAuthors(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"", nil},
		{"Smith, J.; Doe, A.", []string{"Smith, J.", "Doe, A."}},
		{"Alice Smith, Bob Jones", []string{"Alice Smith", "Bob Jones"}},
		{"Smith, J., Doe, A.", []string{"Smith, J.", "Doe, A."}},
		{"Single Author", []string{"Single Author"}},
	}
	for _, tt := range tests {
		got := ParseAuthors(tt.input)
		if !reflect.DeepEqual(got, tt.expected) {
			t.Errorf("ParseAuthors(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestParseDate(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		ok       bool
	}{
		{"2026-03-02", "2026-03-02", true},
		{"2026-3-2", "2026-03-02", true},
		{"2026-03-2", "2026-03-02", true},
		{"2026-3-02", "2026-03-02", true},
		{"2026-03-02T15:04:05Z", "2026-03-02", true},
		{"invalid", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := ParseDate(tt.input)
		if ok != tt.ok {
			t.Errorf("ParseDate(%q) ok = %v, want %v", tt.input, ok, tt.ok)
			continue
		}
		if ok && got.Format("2006-01-02") != tt.expected {
			t.Errorf("ParseDate(%q) = %s, want %s", tt.input, got.Format("2006-01-02"), tt.expected)
		}
	}
}

func TestSharedClient(t *testing.T) {
	mockJSON := `{
		"messages": [{"status": "ok"}],
		"collection": [
			{
				"doi": "10.1101/2026.01.01.000001",
				"title": "Test Paper",
				"authors": "Author, A.; Author, B.",
				"abstract": "Test Abstract",
				"date": "2026-01-01",
				"category": "Genomics",
				"version": "1"
			}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockJSON))
	}))
	defer server.Close()

	client := NewClient(
		WithServer("biorxiv"),
		WithBaseURL(server.URL),
		WithHTTPClient(&http.Client{Timeout: 5 * time.Second}),
	)

	papers, err := client.FetchPapers(context.Background(), []string{"Genomics"})
	if err != nil {
		t.Fatalf("FetchPapers failed: %v", err)
	}

	if len(papers) != 1 {
		t.Fatalf("expected 1 paper, got %d", len(papers))
	}

	p := papers[0]
	if p.Title != "Test Paper" {
		t.Errorf("expected 'Test Paper', got %q", p.Title)
	}
	if p.URL != "https://www.biorxiv.org/content/10.1101/2026.01.01.000001v1" {
		t.Errorf("expected web page URL, got %q", p.URL)
	}
	if p.PDFURL != "https://www.biorxiv.org/content/10.1101/2026.01.01.000001v1.full.pdf" {
		t.Errorf("expected PDF URL, got %q", p.PDFURL)
	}
}
