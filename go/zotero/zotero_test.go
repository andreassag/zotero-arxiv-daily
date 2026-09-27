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
	"net/http"
	"net/http/httptest"
	"testing"
)

const mockCollectionsJSON = `[
  {
    "key": "COL1",
    "data": {
      "name": "survey",
      "parentCollection": false
    }
  },
  {
    "key": "COL2",
    "data": {
      "name": "topic-a",
      "parentCollection": "COL1"
    }
  }
]`

const mockItemsJSON = `[
  {
    "key": "ITEM1",
    "data": {
      "title": "Zotero Title 1",
      "abstractNote": "Abstract 1",
      "dateAdded": "2026-01-15T10:00:00Z",
      "collections": ["COL2"]
    }
  },
  {
    "key": "ITEM2",
    "data": {
      "title": "Zotero Title 2",
      "abstractNote": "",
      "dateAdded": "2026-02-20T12:00:00Z",
      "collections": ["COL1"]
    }
  }
]`

func TestZoteroFetchCorpus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		switch r.URL.Path {
		case "/users/12345/collections", "/users/12345/collections/":
			_, _ = w.Write([]byte(mockCollectionsJSON))
		case "/users/12345/items":
			_, _ = w.Write([]byte(mockItemsJSON))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient("12345", "fake-key", WithBaseURL(server.URL))

	corpus, err := client.FetchCorpus(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// ITEM2 has an empty abstract and should be filtered out
	if len(corpus) != 1 {
		t.Fatalf("expected 1 paper in corpus, got %d", len(corpus))
	}

	paper := corpus[0]
	if paper.Title != "Zotero Title 1" {
		t.Errorf("expected 'Zotero Title 1', got '%s'", paper.Title)
	}
	if paper.Abstract != "Abstract 1" {
		t.Errorf("expected 'Abstract 1', got '%s'", paper.Abstract)
	}
	if len(paper.Paths) != 1 || paper.Paths[0] != "survey/topic-a" {
		t.Errorf("expected collection path 'survey/topic-a', got %v", paper.Paths)
	}
}
