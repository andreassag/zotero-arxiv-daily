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

package retriever

import (
	"context"
	"fmt"

	"github.com/exTerEX/zotero-arxiv-daily/go/arxiv/arxiv"
	"github.com/exTerEX/zotero-arxiv-daily/go/internal/config"
	"github.com/exTerEX/zotero-arxiv-daily/go/internal/model"
)

type ArxivRetriever struct {
	client           *arxiv.Client
	categories       []string
	includeCrossList bool
}

func init() {
	Register("arxiv", func(cfg *config.Config) (Retriever, error) {
		client := arxiv.NewClient(arxiv.WithDebug(cfg.Executor.Debug))
		return &ArxivRetriever{
			client:           client,
			categories:       cfg.Source.Arxiv.Category,
			includeCrossList: cfg.Source.Arxiv.IncludeCrossList,
		}, nil
	})
}

func (r *ArxivRetriever) RetrievePapers(ctx context.Context) ([]model.Paper, error) {
	if len(r.categories) == 0 {
		return nil, fmt.Errorf("no categories specified for arxiv retriever")
	}

	papers, err := r.client.FetchPapers(ctx, r.categories, r.includeCrossList)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch arXiv papers: %w", err)
	}

	var results []model.Paper
	for _, p := range papers {
		results = append(results, model.Paper{
			Source:   "arxiv",
			Title:    p.Title,
			Authors:  p.Authors,
			Abstract: p.Abstract,
			URL:      p.URL,
			PDFURL:   p.PDFURL,
		})
	}
	return results, nil
}
