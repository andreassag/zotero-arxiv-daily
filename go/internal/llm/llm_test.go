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

package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const mockEmbeddingResponse = `{
  "object": "list",
  "data": [
    {
      "object": "embedding",
      "index": 0,
      "embedding": [0.1, 0.2, 0.3]
    },
    {
      "object": "embedding",
      "index": 1,
      "embedding": [0.4, 0.5, 0.6]
    }
  ],
  "model": "text-embedding-3-large",
  "usage": {
    "prompt_tokens": 8,
    "total_tokens": 8
  }
}`

const mockTLDRResponse = `{
  "id": "chatcmpl-123",
  "object": "chat.completion",
  "created": 1677652288,
  "model": "gpt-4o-mini",
  "choices": [{
    "index": 0,
    "message": {
      "role": "assistant",
      "content": "This paper does X and Y."
    },
    "finish_reason": "stop"
  }],
  "usage": {
    "prompt_tokens": 9,
    "completion_tokens": 12,
    "total_tokens": 21
  }
}`

const mockAffiliationsResponse = `{
  "id": "chatcmpl-124",
  "object": "chat.completion",
  "created": 1677652289,
  "model": "gpt-4o-mini",
  "choices": [{
    "index": 0,
    "message": {
      "role": "assistant",
      "content": "Here is the list: [\"University of Washington\", \"Stanford University\"]"
    },
    "finish_reason": "stop"
  }],
  "usage": {
    "prompt_tokens": 9,
    "completion_tokens": 12,
    "total_tokens": 21
  }
}`

func TestLLMGenerateTLDR(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockTLDRResponse))
	}))
	defer server.Close()

	client := NewClient("fake-key", server.URL)
	tldr, err := client.GenerateTLDR(context.Background(), "Title", "Abstract", "English", "gpt-4o-mini", 150)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if tldr != "This paper does X and Y." {
		t.Errorf("expected 'This paper does X and Y.', got '%s'", tldr)
	}
}

func TestLLMExtractAffiliations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockAffiliationsResponse))
	}))
	defer server.Close()

	client := NewClient("fake-key", server.URL)
	affs, err := client.ExtractAffiliations(context.Background(), "Paper Title", []string{"Author One"}, "Paper Abstract", "Full text starting...", "gpt-4o-mini")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(affs) != 2 || affs[0] != "University of Washington" || affs[1] != "Stanford University" {
		t.Errorf("expected UW and Stanford, got %v", affs)
	}
}

func TestLLMGetEmbeddings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockEmbeddingResponse))
	}))
	defer server.Close()

	client := NewClient("fake-key", server.URL)
	embeds, err := client.GetEmbeddings(context.Background(), []string{"Text 1", "Text 2"}, "text-embedding-3-large", 64)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(embeds) != 2 {
		t.Fatalf("expected 2 embedding vectors, got %d", len(embeds))
	}

	if embeds[0][0] != 0.1 || embeds[0][1] != 0.2 || embeds[0][2] != 0.3 {
		t.Errorf("incorrect embedding values at index 0: %v", embeds[0])
	}
}
