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

package executor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/exTerEX/zotero-arxiv-daily/go/internal/model"
)

func TestParseMetaInstitutions(t *testing.T) {
	htmlSample := `
<!DOCTYPE html>
<html>
<head>
    <meta name="citation_title" content="A Great Biology Paper" />
    <meta name="citation_author" content="Alice Smith" />
    <meta name="citation_author_institution" content="Department of Genetics, Stanford University" />
    <meta name="citation_author" content="Bob Jones" />
    <meta content="Harvard Medical School; Broad Institute" name="citation_author_institution" />
    <meta name='citation_author_institution' content='Stanford University.' />
</head>
<body></body>
</html>`

	got := parseMetaInstitutions(htmlSample)
	expected := []string{
		"Department of Genetics, Stanford University",
		"Harvard Medical School",
		"Broad Institute",
		"Stanford University",
	}

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("parseMetaInstitutions() = %v, want %v", got, expected)
	}
}

func TestParseArxivHTML(t *testing.T) {
	htmlSample := `
<div class="ltx_authors">
    <span class="ltx_creator ltx_role_author">John Doe</span>
    <span class="ltx_contact ltx_role_affiliation"><span class="ltx_contact_name">Affiliation: </span>University of California, Davis</span>
</div>`

	got := parseArxivHTML(htmlSample)
	expected := []string{"University of California, Davis"}

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("parseArxivHTML() = %v, want %v", got, expected)
	}
}

func TestExtractAffiliations_HTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`
<html>
<head>
<meta name="citation_author_institution" content="Yale University" />
<meta name="citation_author_institution" content="University of Pennsylvania" />
</head>
</html>`))
	}))
	defer server.Close()

	paper := &model.Paper{
		Title:  "Test Paper",
		URL:    server.URL,
		PDFURL: server.URL + ".pdf",
	}

	affs := ExtractAffiliations(context.Background(), server.Client(), nil, paper, "test-model")
	expected := []string{"Yale University", "University of Pennsylvania"}

	if !reflect.DeepEqual(affs, expected) {
		t.Errorf("ExtractAffiliations() = %v, want %v", affs, expected)
	}
}
