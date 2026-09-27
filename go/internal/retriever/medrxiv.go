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

	"github.com/exTerEX/zotero-arxiv-daily/go/arxiv/medrxiv"
	"github.com/exTerEX/zotero-arxiv-daily/go/internal/config"
	"github.com/exTerEX/zotero-arxiv-daily/go/internal/model"
)

type MedrxivRetriever struct {
	client     *medrxiv.Client
	categories []string
}

func init() {
	Register("medrxiv", func(cfg *config.Config) (Retriever, error) {
		client := medrxiv.NewClient(medrxiv.WithDebug(cfg.Executor.Debug))
		return &MedrxivRetriever{
			client:     client,
			categories: cfg.Source.Medrxiv.Category,
		}, nil
	})
}

func (r *MedrxivRetriever) RetrievePapers(ctx context.Context) ([]model.Paper, error) {
	if len(r.categories) == 0 {
		return nil, fmt.Errorf("no categories specified for medrxiv retriever")
	}

	papers, err := r.client.FetchPapers(ctx, r.categories)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch medRxiv papers: %w", err)
	}

	var results []model.Paper
	for _, p := range papers {
		results = append(results, model.Paper{
			Source:   "medrxiv",
			Title:    p.Title,
			Authors:  p.Authors,
			Abstract: p.Abstract,
			URL:      p.URL,
			PDFURL:   p.PDFURL,
		})
	}
	return results, nil
}
