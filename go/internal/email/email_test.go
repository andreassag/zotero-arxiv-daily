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

package email

import (
	"strings"
	"testing"

	"github.com/exTerEX/zotero-arxiv-daily/go/internal/model"
)

func TestRenderEmail(t *testing.T) {
	papers := []model.Paper{
		{
			Title:        "Genome engineering in *Escherichia coli* and *Homo sapiens*",
			Authors:      []string{"Alice Smith", "Bob Jones"},
			Affiliations: []string{"Stanford University", "Harvard Medical School"},
			Score:        7.8,
			TLDR:         "This study optimizes CRISPR targeting in *E. coli* (*p* < 0.01).",
			Abstract:     "We developed a synthetic biology approach in *Escherichia coli* with **high** precision.",
			URL:          "https://www.biorxiv.org/content/10.1101/2026.01.01.123456v1",
			PDFURL:       "https://www.biorxiv.org/content/10.1101/2026.01.01.123456v1.full.pdf",
		},
		{
			Title:        "A study on deep learning",
			Authors:      []string{"Charlie Brown"},
			Affiliations: nil, // empty affiliations
			Score:        6.5,
			TLDR:         "Machine learning model for protein structure.",
			Abstract:     "Machine learning model for protein structure.",
			URL:          "https://arxiv.org/abs/2301.00001",
			PDFURL:       "https://arxiv.org/pdf/2301.00001",
		},
	}

	html := RenderEmail(papers)

	// 1. Check title markdown rendering
	if !strings.Contains(html, "<em>Escherichia coli</em> and <em>Homo sapiens</em>") {
		t.Errorf("Expected italic species in title to be rendered as <em>, got html:\n%s", html)
	}

	// 2. Check TLDR markdown rendering
	if !strings.Contains(html, "<em>E. coli</em> (<em>p</em> &lt; 0.01)") && !strings.Contains(html, "<em>E. coli</em> (<em>p</em> < 0.01)") {
		t.Errorf("Expected italic species in TLDR to be rendered as <em>, got html:\n%s", html)
	}

	// 3. Check Abstract markdown rendering
	if !strings.Contains(html, "<em>Escherichia coli</em> with <strong>high</strong> precision") {
		t.Errorf("Expected abstract markdown to be rendered as <em> and <strong>, got html:\n%s", html)
	}

	// 4. Check affiliations rendered
	if !strings.Contains(html, "Stanford University; Harvard Medical School") {
		t.Errorf("Expected affiliations to be rendered, got html:\n%s", html)
	}
	if !strings.Contains(html, "🏛️") {
		t.Errorf("Expected institution icon 🏛️ in affiliations block, got html:\n%s", html)
	}

	// 5. Check no "Unknown Affiliation" placeholder
	if strings.Contains(html, "Unknown Affiliation") {
		t.Errorf("Expected 'Unknown Affiliation' to be omitted when affiliations are nil, got html:\n%s", html)
	}

	// 6. Check collapsible abstract details
	if !strings.Contains(html, "<details") || !strings.Contains(html, "<summary") {
		t.Errorf("Expected <details><summary> for abstract, got html:\n%s", html)
	}
}
