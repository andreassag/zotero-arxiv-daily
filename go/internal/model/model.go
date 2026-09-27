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

package model

import (
	"time"
)

// Paper is the internal representation of a paper retrieved from any source.
type Paper struct {
	Source       string   `json:"source"`
	Title        string   `json:"title"`
	Authors      []string `json:"authors"`
	Abstract     string   `json:"abstract"`
	URL          string   `json:"url"`
	PDFURL       string   `json:"pdf_url"`
	FullText     string   `json:"full_text,omitempty"`
	TLDR         string   `json:"tldr,omitempty"`
	Affiliations []string `json:"affiliations,omitempty"`
	Score        float64  `json:"score"`
}

// CorpusPaper is the internal representation of a paper stored in the Zotero library.
type CorpusPaper struct {
	Title     string    `json:"title"`
	Abstract  string    `json:"abstract"`
	AddedDate time.Time `json:"added_date"`
	Paths     []string  `json:"paths"`
}
