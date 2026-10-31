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

package biorxiv

import (
	"github.com/exTerEX/zotero-arxiv-daily/go/arxiv/shared"
)

// Paper represents a bioRxiv paper.
type Paper = shared.Paper

// Client fetches papers from the bioRxiv API (with RSS fallback).
type Client = shared.Client

// Option is a functional option for configuring a Client.
type Option = shared.Option

var (
	WithHTTPClient = shared.WithHTTPClient
	WithBaseURL    = shared.WithBaseURL
	WithRSSBaseURL = shared.WithRSSBaseURL
	WithDebug      = shared.WithDebug
	WithServer     = shared.WithServer

	parseDate    = shared.ParseDate
	cleanText    = shared.CleanText
	parseAuthors = shared.ParseAuthors
)

// NewClient creates a new Client configured for bioRxiv.
func NewClient(opts ...Option) *Client {
	defaults := []Option{
		shared.WithServer("biorxiv"),
		shared.WithBaseURL("https://api.biorxiv.org/details"),
		shared.WithRSSBaseURL("https://connect.biorxiv.org/biorxiv_xml.php"),
	}
	allOpts := append(defaults, opts...)
	return shared.NewClient(allOpts...)
}
