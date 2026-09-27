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
	linkRegex       = regexp.MustCompile(`\[([^\]]+)\]\(([^()\s]+(?:\([^()\s]*\)[^()\s]*)*)\)`)
)

func isSafeLinkURL(rawURL string) bool {
	lower := strings.ToLower(strings.TrimSpace(rawURL))
	return strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "mailto:")
}

// MarkdownToHTML converts basic Markdown formatting (bold, italic, species names, code, links, lists) into sanitized, email-safe HTML.
func MarkdownToHTML(input string) string {
	if strings.TrimSpace(input) == "" {
		return ""
	}

	text := input

	// Temporarily preserve safe known formatting tags from PubMed/bioRxiv HTML
	const (
		phEmStart     = "\x00_EM_START_\x00"
		phEmEnd       = "\x00_EM_END_\x00"
		phStrongStart = "\x00_STRONG_START_\x00"
		phStrongEnd   = "\x00_STRONG_END_\x00"
	)

	text = strings.ReplaceAll(text, "<i>", phEmStart)
	text = strings.ReplaceAll(text, "</i>", phEmEnd)
	text = strings.ReplaceAll(text, "<em>", phEmStart)
	text = strings.ReplaceAll(text, "</em>", phEmEnd)
	text = strings.ReplaceAll(text, "<b>", phStrongStart)
	text = strings.ReplaceAll(text, "</b>", phStrongEnd)
	text = strings.ReplaceAll(text, "<strong>", phStrongStart)
	text = strings.ReplaceAll(text, "</strong>", phStrongEnd)

	// HTML-escape to neutralize any malicious tags (<script>, <iframe>, <img>, etc.)
	text = html.EscapeString(text)

	// Restore preserved safe emphasis tags
	text = strings.ReplaceAll(text, phEmStart, "<em>")
	text = strings.ReplaceAll(text, phEmEnd, "</em>")
	text = strings.ReplaceAll(text, phStrongStart, "<strong>")
	text = strings.ReplaceAll(text, phStrongEnd, "</strong>")

	// 1. Inline Code
	text = codeRegex.ReplaceAllStringFunc(text, func(m string) string {
		inner := m[1 : len(m)-1]
		return fmt.Sprintf(`<code style="background-color: #f1f5f9; padding: 2px 5px; border-radius: 4px; font-family: monospace; font-size: 88%%;">%s</code>`, inner)
	})

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

	// 5. Links [text](url) - restricted to safe schemes (https, http, mailto) with attribute escaping
	text = linkRegex.ReplaceAllStringFunc(text, func(m string) string {
		sub := linkRegex.FindStringSubmatch(m)
		if len(sub) != 3 {
			return m
		}
		linkText := sub[1]
		targetURL := sub[2]
		if !isSafeLinkURL(targetURL) {
			return linkText
		}
		return fmt.Sprintf(`<a href="%s" style="color: #2563eb; text-decoration: underline;" target="_blank">%s</a>`, html.EscapeString(targetURL), linkText)
	})

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
