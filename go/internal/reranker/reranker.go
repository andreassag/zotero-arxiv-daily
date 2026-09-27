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
	"fmt"
	"math"
	"sort"

	"github.com/exTerEX/zotero-arxiv-daily/go/internal/llm"
	"github.com/exTerEX/zotero-arxiv-daily/go/internal/model"
)

type Reranker struct {
	llmClient *llm.Client
	modelName string
	batchSize int
}

func New(llmClient *llm.Client, modelName string, batchSize int) *Reranker {
	return &Reranker{
		llmClient: llmClient,
		modelName: modelName,
		batchSize: batchSize,
	}
}

// Rerank scores and sorts the candidates based on cosine similarity to the corpus,
// applying a logarithmic time-decay weighting to older corpus papers.
func (r *Reranker) Rerank(ctx context.Context, candidates []model.Paper, corpus []model.CorpusPaper) ([]model.Paper, error) {
	if len(candidates) == 0 || len(corpus) == 0 {
		for i := range candidates {
			candidates[i].Score = 0.0
		}
		return candidates, nil
	}

	// 1. Sort corpus by date descending (newest first)
	sortedCorpus := make([]model.CorpusPaper, len(corpus))
	copy(sortedCorpus, corpus)
	sort.Slice(sortedCorpus, func(i, j int) bool {
		return sortedCorpus[i].AddedDate.After(sortedCorpus[j].AddedDate)
	})

	// 2. Compute time decay weights
	weights := make([]float64, len(sortedCorpus))
	var weightSum float64
	for i := range sortedCorpus {
		weights[i] = 1.0 / (1.0 + math.Log10(float64(i+1)))
		weightSum += weights[i]
	}
	// Normalize weights
	for i := range weights {
		weights[i] /= weightSum
	}

	// 3. Fetch embeddings for candidates and corpus abstracts
	var texts []string
	for _, c := range candidates {
		texts = append(texts, c.Abstract)
	}
	for _, c := range sortedCorpus {
		texts = append(texts, c.Abstract)
	}

	embeddings, err := r.llmClient.GetEmbeddings(ctx, texts, r.modelName, r.batchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch embeddings: %w", err)
	}

	candidateEmbeds := embeddings[:len(candidates)]
	corpusEmbeds := embeddings[len(candidates):]

	// 4. Normalize embedding vectors
	normCandidateEmbeds := make([][]float64, len(candidateEmbeds))
	for i, emb := range candidateEmbeds {
		normCandidateEmbeds[i] = normalize(emb)
	}

	normCorpusEmbeds := make([][]float64, len(corpusEmbeds))
	for i, emb := range corpusEmbeds {
		normCorpusEmbeds[i] = normalize(emb)
	}

	// 5. Calculate scores with time decay
	scoredCandidates := make([]model.Paper, len(candidates))
	copy(scoredCandidates, candidates)

	for j := range scoredCandidates {
		var totalSim float64
		for k := range sortedCorpus {
			sim := dotProduct(normCandidateEmbeds[j], normCorpusEmbeds[k])
			totalSim += sim * weights[k]
		}
		scoredCandidates[j].Score = totalSim * 10.0
	}

	// 6. Sort by score descending
	sort.Slice(scoredCandidates, func(i, j int) bool {
		return scoredCandidates[i].Score > scoredCandidates[j].Score
	})

	return scoredCandidates, nil
}

func dotProduct(v1, v2 []float64) float64 {
	if len(v1) != len(v2) || len(v1) == 0 {
		return 0
	}
	var sum float64
	for i := range v1 {
		sum += v1[i] * v2[i]
	}
	return sum
}

func norm(v []float64) float64 {
	var sum float64
	for _, val := range v {
		sum += val * val
	}
	return math.Sqrt(sum)
}

func normalize(v []float64) []float64 {
	n := norm(v)
	if n == 0 {
		return make([]float64, len(v))
	}
	res := make([]float64, len(v))
	for i, val := range v {
		res[i] = val / n
	}
	return res
}
