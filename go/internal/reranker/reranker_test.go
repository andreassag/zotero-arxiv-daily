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

package reranker

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/exTerEX/zotero-arxiv-daily/go/internal/llm"
	"github.com/exTerEX/zotero-arxiv-daily/go/internal/model"
)

func TestMathHelpers(t *testing.T) {
	// dotProduct
	v1 := []float64{1.0, 2.0, 3.0}
	v2 := []float64{4.0, 5.0, 6.0}
	if got := dotProduct(v1, v2); got != 32.0 {
		t.Errorf("dotProduct = %f, want 32.0", got)
	}
	if got := dotProduct(nil, v2); got != 0 {
		t.Errorf("dotProduct with nil = %f, want 0", got)
	}

	// norm
	v := []float64{3.0, 4.0}
	if got := norm(v); got != 5.0 {
		t.Errorf("norm = %f, want 5.0", got)
	}

	// normalize
	normV := normalize(v)
	if math.Abs(normV[0]-0.6) > 1e-6 || math.Abs(normV[1]-0.8) > 1e-6 {
		t.Errorf("normalize = %v, want [0.6, 0.8]", normV)
	}

	// normalize zero vector
	zeroV := []float64{0.0, 0.0}
	normZero := normalize(zeroV)
	if normZero[0] != 0 || normZero[1] != 0 {
		t.Errorf("normalize zero vector = %v, want [0, 0]", normZero)
	}
}

func TestRerankerEmpty(t *testing.T) {
	client := llm.NewClient("fake", "http://fake")
	r := New(client, "test-model", 32)

	// Empty candidates
	papers, err := r.Rerank(context.Background(), nil, []model.CorpusPaper{{Title: "T"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(papers) != 0 {
		t.Errorf("expected 0 papers, got %d", len(papers))
	}

	// Empty corpus
	candidates := []model.Paper{{Title: "C1", Score: 5.0}}
	papers, err = r.Rerank(context.Background(), candidates, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(papers) != 1 || papers[0].Score != 0.0 {
		t.Errorf("expected candidates with score 0.0, got: %+v", papers)
	}
}

func TestRerankerScoringAndDebugData(t *testing.T) {
	tmpDir := t.TempDir()
	origWd, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer os.Chdir(origWd)

	t.Setenv("DEBUG", "true")

	// Mock embedding server: 2 candidates, 1 corpus paper -> 3 texts
	mockResponse := `{
		"object": "list",
		"data": [
			{"object": "embedding", "index": 0, "embedding": [1.0, 0.0]},
			{"object": "embedding", "index": 1, "embedding": [0.0, 1.0]},
			{"object": "embedding", "index": 2, "embedding": [1.0, 0.0]}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	llmClient := llm.NewClient("fake", server.URL)
	ranker := New(llmClient, "test-model", 32)

	candidates := []model.Paper{
		{Title: "Unrelated", Abstract: "Unrelated text"},
		{Title: "Highly Related", Abstract: "Identical direction"},
	}
	corpus := []model.CorpusPaper{
		{Title: "Corpus Paper", Abstract: "Corpus text", AddedDate: time.Now()},
	}

	scored, err := ranker.Rerank(context.Background(), candidates, corpus)
	if err != nil {
		t.Fatalf("Rerank failed: %v", err)
	}

	if len(scored) != 2 {
		t.Fatalf("expected 2 scored papers, got %d", len(scored))
	}

	// First paper should have higher score (direction [1,0] matches corpus [1,0])
	if scored[0].Title != "Unrelated" {
		t.Errorf("expected 'Unrelated' with dot product 1.0 to rank first, got %q", scored[0].Title)
	}
	if scored[0].Score <= scored[1].Score {
		t.Errorf("expected scored[0].Score (%f) > scored[1].Score (%f)", scored[0].Score, scored[1].Score)
	}

	// Verify debug files were saved
	if _, err := os.Stat("outputs/embedding_input_s1.json"); os.IsNotExist(err) {
		t.Errorf("outputs/embedding_input_s1.json was not created")
	}
	if _, err := os.Stat("outputs/embedding_input_s2.json"); os.IsNotExist(err) {
		t.Errorf("outputs/embedding_input_s2.json was not created")
	}
}
