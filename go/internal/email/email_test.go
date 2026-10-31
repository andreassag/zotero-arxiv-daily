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
	"bufio"
	"io"
	"mime/quotedprintable"
	"net"
	"reflect"
	"strings"
	"testing"

	"github.com/exTerEX/zotero-arxiv-daily/go/internal/config"
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

func TestRenderEmptyEmail(t *testing.T) {
	html := RenderEmail(nil)
	if !strings.Contains(html, "No Papers Found Today") {
		t.Errorf("Expected 'No Papers Found Today' in empty email, got: %s", html)
	}
	if !strings.Contains(html, "☕") {
		t.Errorf("Expected coffee emoji in empty email, got: %s", html)
	}
}

func TestBuildMIMEMessageLineLengthAndQuotedPrintable(t *testing.T) {
	// Create a huge continuous line (> 5000 chars) simulating scientific abstracts without newlines
	hugeAbstract := strings.Repeat("ArbitraryLongBiologicalSequenceAndScientificAbstractContentWithoutAnyNewlines ", 70)
	body := "<html><body><h1>Test</h1><p>" + hugeAbstract + "</p><p>Emoji: 🏛️ ⭐ ☕</p></body></html>"

	sender := "sender@example.com"
	receiver := "receiver@example.com"
	subject := "Daily arXiv digest with unicode: 🚀"

	msg, err := buildMIMEMessage(sender, receiver, subject, body)
	if err != nil {
		t.Fatalf("buildMIMEMessage failed: %v", err)
	}

	// 1. Verify headers
	msgStr := string(msg)
	if !strings.Contains(msgStr, "Content-Transfer-Encoding: quoted-printable") {
		t.Errorf("Expected Content-Transfer-Encoding: quoted-printable in headers")
	}
	if !strings.Contains(msgStr, "MIME-Version: 1.0") {
		t.Errorf("Expected MIME-Version: 1.0 in headers")
	}

	// 2. Verify RFC 5321/5322 line length limit: every line must be <= 78 bytes (excluding \r\n, <= 76)
	lines := strings.Split(msgStr, "\r\n")
	for idx, line := range lines {
		// If line is empty or trailing split, skip
		if idx == len(lines)-1 && line == "" {
			continue
		}
		if len(line) > 78 {
			t.Errorf("Line %d exceeds 78 characters (len=%d): %q", idx, len(line), line)
		}
	}

	// 3. Verify Quoted-Printable roundtrip decoding matches original HTML body
	headerEnd := strings.Index(msgStr, "\r\n\r\n")
	if headerEnd == -1 {
		t.Fatalf("MIME message missing header-body boundary")
	}
	encodedBody := msgStr[headerEnd+4:]
	qpReader := quotedprintable.NewReader(strings.NewReader(encodedBody))
	decodedBytes, err := io.ReadAll(qpReader)
	if err != nil {
		t.Fatalf("Failed to decode quoted-printable body: %v", err)
	}

	if string(decodedBytes) != body {
		t.Errorf("Quoted-printable decoded body does not match original body.\nGot: %q\nWant: %q", string(decodedBytes), body)
	}
}

func TestBuildMIMEMessageHeaderSanitization(t *testing.T) {
	// Attempt CRLF injection in sender, receiver, and subject
	sender := "sender@example.com\r\nBcc: evil-sender@example.com"
	receiver := "receiver@example.com\r\nCc: evil-receiver@example.com"
	subject := "Test Subject\r\nInjected-Header: evil"
	body := "<p>Test</p>"

	msg, err := buildMIMEMessage(sender, receiver, subject, body)
	if err != nil {
		t.Fatalf("buildMIMEMessage failed: %v", err)
	}

	msgStr := string(msg)
	headerEnd := strings.Index(msgStr, "\r\n\r\n")
	if headerEnd == -1 {
		t.Fatalf("Missing header-body separator")
	}
	headers := msgStr[:headerEnd]

	// Check that injected headers are not present on their own lines
	if strings.Contains(headers, "\r\nBcc:") || strings.Contains(headers, "\r\nCc:") || strings.Contains(headers, "\r\nInjected-Header:") {
		t.Errorf("CRLF header injection was not prevented: %s", headers)
	}
}

func TestAddressFormatting(t *testing.T) {
	// Plain email
	if got := formatAddressHeader("GitHub Action", "bot@example.com"); got != "GitHub Action <bot@example.com>" {
		t.Errorf("Expected 'GitHub Action <bot@example.com>', got: %s", got)
	}

	// Already formatted name and email
	if got := formatAddressHeader("GitHub Action", "Custom Bot <custom@example.com>"); got != "Custom Bot <custom@example.com>" {
		t.Errorf("Expected 'Custom Bot <custom@example.com>', got: %s", got)
	}

	// Extraction for SMTP MAIL FROM / RCPT TO
	if got := extractEmailAddress("Custom Bot <custom@example.com>"); got != "custom@example.com" {
		t.Errorf("Expected 'custom@example.com', got: %s", got)
	}
}

func TestMultipleRecipients(t *testing.T) {
	// Comma separated
	addrs := extractEmailAddresses("alice@example.com, bob@example.com")
	want := []string{"alice@example.com", "bob@example.com"}
	if !reflect.DeepEqual(addrs, want) {
		t.Errorf("extractEmailAddresses = %v, want %v", addrs, want)
	}

	// Semicolon separated
	addrsSemi := extractEmailAddresses("alice@example.com; bob@example.com")
	if !reflect.DeepEqual(addrsSemi, want) {
		t.Errorf("extractEmailAddresses with semicolon = %v, want %v", addrsSemi, want)
	}

	// With names
	addrsWithNames := extractEmailAddresses("Alice <alice@example.com>, Bob <bob@example.com>")
	if !reflect.DeepEqual(addrsWithNames, want) {
		t.Errorf("extractEmailAddresses with names = %v, want %v", addrsWithNames, want)
	}

	// Header formatting
	header := formatAddressListHeader("You", "alice@example.com, Bob <bob@example.com>")
	if !strings.Contains(header, "You <alice@example.com>") || !strings.Contains(header, "Bob <bob@example.com>") {
		t.Errorf("unexpected header format: %s", header)
	}
}

func TestSendEmailMockSMTP(t *testing.T) {
	// Start local mock SMTP server (port 25-like plain connection)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port

	var receivedRcpts []string
	var receivedData string
	done := make(chan struct{})

	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		reader := bufio.NewReader(conn)
		writer := bufio.NewWriter(conn)

		sendLine := func(line string) {
			_, _ = writer.WriteString(line + "\r\n")
			_ = writer.Flush()
		}

		sendLine("220 mock.smtp ready")

		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				break
			}
			line = strings.TrimRight(line, "\r\n")
			upper := strings.ToUpper(line)

			if strings.HasPrefix(upper, "EHLO") || strings.HasPrefix(upper, "HELO") {
				sendLine("250-mock.smtp")
				sendLine("250 8BITMIME")
			} else if strings.HasPrefix(upper, "MAIL FROM:") {
				sendLine("250 OK")
			} else if strings.HasPrefix(upper, "RCPT TO:") {
				receivedRcpts = append(receivedRcpts, line)
				sendLine("250 OK")
			} else if upper == "DATA" {
				sendLine("354 Start mail input; end with <CRLF>.<CRLF>")
				var dataBuf strings.Builder
				for {
					dLine, dErr := reader.ReadString('\n')
					if dErr != nil {
						break
					}
					if dLine == ".\r\n" || dLine == ".\n" {
						break
					}
					dataBuf.WriteString(dLine)
				}
				receivedData = dataBuf.String()
				sendLine("250 OK: queued as 12345")
			} else if upper == "QUIT" {
				sendLine("221 Bye")
				break
			} else {
				sendLine("500 Unknown command")
			}
		}
	}()

	cfg := config.EmailConfig{
		Sender:     "sender@example.com",
		Receiver:   "user1@example.com, user2@example.com",
		SMTPServer: "127.0.0.1",
		SMTPPort:   port,
	}

	err = SendEmail(cfg, "<h1>Hello World</h1><p>This is a test email digest.</p>")
	if err != nil {
		t.Fatalf("SendEmail failed: %v", err)
	}

	<-done

	// Verify both recipients received RCPT TO
	if len(receivedRcpts) != 2 {
		t.Errorf("expected 2 RCPT TO commands, got %d: %v", len(receivedRcpts), receivedRcpts)
	}

	// Verify received data is Quoted-Printable encoded and contains expected content
	if !strings.Contains(receivedData, "Content-Transfer-Encoding: quoted-printable") {
		t.Errorf("expected quoted-printable header in received data")
	}
	if !strings.Contains(receivedData, "Hello World") {
		t.Errorf("expected email body content in received data")
	}
}
