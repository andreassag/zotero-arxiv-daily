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
	"fmt"
	"html"
	"regexp"
	"strings"
)

var (
	boldItalicRegex = regexp.MustCompile(`(?:\*\*\*|___)([^\*_]+?)(?:\*\*\*|___)`)
	boldRegex       = regexp.MustCompile(`(?:\*\*|__)([^\*_]+?)(?:\*\*|__)`)
	italicRegex     = regexp.MustCompile(`(?:^|[\s\(\[\{<>\.,;:!?])(?:\*|_)([^\*_\n]+?)(?:\*|_)(?:$|[\s\)\]\}<>\.,;:!?])`)
	codeRegex       = regexp.MustCompile("`([^`\n]+?)`")
	linkRegex       = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
)

// MarkdownToHTML converts basic Markdown formatting (bold, italic, species names, code, links, lists) into email-safe HTML.
func MarkdownToHTML(input string) string {
	if strings.TrimSpace(input) == "" {
		return ""
	}

	// Normalize HTML entities so we start from clean unicode
	text := html.UnescapeString(input)

	// Normalize pre-existing HTML tags if present
	text = strings.ReplaceAll(text, "<i>", "<em>")
	text = strings.ReplaceAll(text, "</i>", "</em>")
	text = strings.ReplaceAll(text, "<b>", "<strong>")
	text = strings.ReplaceAll(text, "</b>", "</strong>")

	// 1. Inline Code
	text = codeRegex.ReplaceAllString(text, `<code style="background-color: #f1f5f9; padding: 2px 5px; border-radius: 4px; font-family: monospace; font-size: 88%;">$1</code>`)

	// 2. Bold + Italic (***text*** or ___text___)
	text = boldItalicRegex.ReplaceAllString(text, `<strong><em>$1</em></strong>`)

	// 3. Bold (**text** or __text__)
	text = boldRegex.ReplaceAllString(text, `<strong>$1</strong>`)

	// 4. Italic (*text* or _text_) - preserves punctuation boundaries (e.g. *E. coli*, *S. cerevisiae*)
	for pass := 0; pass < 5; pass++ {
		newText := italicRegex.ReplaceAllStringFunc(text, func(m string) string {
			prefix := ""
			suffix := ""
			content := m

			first := m[0]
			if first != '*' && first != '_' {
				prefix = string(first)
				content = content[1:]
			}
			last := content[len(content)-1]
			if last != '*' && last != '_' {
				suffix = string(last)
				content = content[:len(content)-1]
			}
			if len(content) >= 2 && (content[0] == '*' || content[0] == '_') && (content[len(content)-1] == '*' || content[len(content)-1] == '_') {
				inner := content[1 : len(content)-1]
				return prefix + "<em>" + inner + "</em>" + suffix
			}
			return m
		})
		if newText == text {
			break
		}
		text = newText
	}

	// 5. Links [text](url)
	text = linkRegex.ReplaceAllString(text, `<a href="$2" style="color: #2563eb; text-decoration: underline;" target="_blank">$1</a>`)

	// 6. Bullet lists
	lines := strings.Split(text, "\n")
	var inList bool
	var processedLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			itemContent := strings.TrimSpace(trimmed[2:])
			if !inList {
				processedLines = append(processedLines, `<ul style="margin: 8px 0; padding-left: 20px;">`)
				inList = true
			}
			processedLines = append(processedLines, fmt.Sprintf(`<li style="margin-bottom: 4px;">%s</li>`, itemContent))
		} else {
			if inList {
				processedLines = append(processedLines, "</ul>")
				inList = false
			}
			processedLines = append(processedLines, line)
		}
	}
	if inList {
		processedLines = append(processedLines, "</ul>")
	}
	text = strings.Join(processedLines, "\n")

	// 7. Line breaks
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\n\n", "<br><br>")
	text = strings.ReplaceAll(text, "\n", " ")

	return text
}
