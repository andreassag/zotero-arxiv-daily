package email

import (
	"strings"
	"testing"
)

func TestMarkdownToHTML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Single species italic",
			input:    "This paper investigates *Homo sapiens* and *Escherichia coli*.",
			expected: "This paper investigates <em>Homo sapiens</em> and <em>Escherichia coli</em>.",
		},
		{
			name:     "Italic with trailing punctuation",
			input:    "Found in *S. cerevisiae*, and *Bacillus subtilis*.",
			expected: "Found in <em>S. cerevisiae</em>, and <em>Bacillus subtilis</em>.",
		},
		{
			name:     "Italic in parentheses",
			input:    "Significant increase (*p* < 0.05) observed.",
			expected: "Significant increase (<em>p</em> < 0.05) observed.",
		},
		{
			name:     "Bold and italic",
			input:    "This is **bold** and *italic* and ***both***.",
			expected: "This is <strong>bold</strong> and <em>italic</em> and <strong><em>both</em></strong>.",
		},
		{
			name:     "Inline code and links",
			input:    "Run `make test` or visit [GitHub](https://github.com).",
			expected: `Run <code style="background-color: #f1f5f9; padding: 2px 5px; border-radius: 4px; font-family: monospace; font-size: 88%;">make test</code> or visit <a href="https://github.com" style="color: #2563eb; text-decoration: underline;" target="_blank">GitHub</a>.`,
		},
		{
			name:     "Consecutive italics",
			input:    "*Homo sapiens* *Pan troglodytes* and *Mus musculus*",
			expected: "<em>Homo sapiens</em> <em>Pan troglodytes</em> and <em>Mus musculus</em>",
		},
		{
			name:     "Pre-existing HTML tags",
			input:    "Already has <i>italic</i> and <b>bold</b>.",
			expected: "Already has <em>italic</em> and <strong>bold</strong>.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MarkdownToHTML(tt.input)
			if !strings.Contains(got, tt.expected) {
				t.Errorf("MarkdownToHTML() =\n%q\nwant to contain\n%q", got, tt.expected)
			}
		})
	}
}
