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
	"crypto/tls"
	"fmt"
	"html"
	"net/smtp"
	"strings"
	"time"

	"github.com/exTerEX/zotero-arxiv-daily/go/internal/config"
	"github.com/exTerEX/zotero-arxiv-daily/go/internal/model"
)

func safeHref(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") {
		return html.EscapeString(trimmed)
	}
	return "#"
}

// RenderEmail generates a beautiful modern material/minimalist HTML email from the list of papers.
func RenderEmail(papers []model.Paper) string {
	if len(papers) == 0 {
		return renderEmptyEmail()
	}

	var cardBlocks []string
	for _, p := range papers {
		// Clean authors list: truncate if > 5
		authors := ""
		if len(p.Authors) <= 5 {
			authors = strings.Join(p.Authors, ", ")
		} else {
			authors = strings.Join(p.Authors[:3], ", ") + ", ..., " + strings.Join(p.Authors[len(p.Authors)-2:], ", ")
		}

		// Clean affiliations
		affBlock := ""
		if len(p.Affiliations) > 0 {
			var escapedAffs []string
			for _, a := range p.Affiliations {
				escapedAffs = append(escapedAffs, html.EscapeString(a))
			}
			var affStr string
			if len(escapedAffs) <= 5 {
				affStr = strings.Join(escapedAffs, "; ")
			} else {
				affStr = strings.Join(escapedAffs[:5], "; ") + ", ..."
			}
			affBlock = fmt.Sprintf(`<div style="font-size: 13px; color: #64748b; margin-top: 4px; line-height: 1.4;"><span style="margin-right: 4px;">🏛️</span>%s</div>`, affStr)
		}

		// Render stars relevance
		stars := getStarsHTML(p.Score)

		// Format Title with Markdown
		formattedTitle := MarkdownToHTML(p.Title)

		// Format TL;DR
		formattedTLDR := MarkdownToHTML(p.TLDR)
		tldrBlock := ""
		if formattedTLDR != "" && strings.TrimSpace(p.TLDR) != strings.TrimSpace(p.Abstract) {
			tldrBlock = fmt.Sprintf(`
			<div style="margin-bottom: 16px; background-color: #f8fafc; border-left: 4px solid #2563eb; padding: 12px 16px; border-radius: 0 8px 8px 0;">
				<div style="font-size: 12px; font-weight: 700; color: #1d4ed8; text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 4px;">TL;DR</div>
				<div style="font-size: 14px; color: #1e293b; line-height: 1.6;">%s</div>
			</div>`, formattedTLDR)
		}

		// Format Abstract
		formattedAbstract := MarkdownToHTML(p.Abstract)
		abstractBlock := ""
		if formattedAbstract != "" {
			if tldrBlock != "" {
				// Collapsible abstract if TL;DR is already shown
				abstractBlock = fmt.Sprintf(`
				<details style="margin-bottom: 16px; border: 1px solid #e2e8f0; border-radius: 8px; background-color: #ffffff; padding: 10px 14px;">
					<summary style="font-size: 13px; font-weight: 600; color: #475569; cursor: pointer; user-select: none;">
						Abstract
					</summary>
					<div style="font-size: 14px; color: #334155; line-height: 1.6; margin-top: 10px; border-top: 1px dashed #cbd5e1; padding-top: 10px;">
						%s
					</div>
				</details>`, formattedAbstract)
			} else {
				// Standalone abstract block if no separate TL;DR
				abstractBlock = fmt.Sprintf(`
				<div style="margin-bottom: 16px;">
					<div style="font-size: 12px; font-weight: 700; color: #475569; text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 4px;">Abstract</div>
					<div style="font-size: 14px; color: #334155; line-height: 1.6;">%s</div>
				</div>`, formattedAbstract)
			}
		}

		// Action buttons
		webButton := ""
		if p.URL != "" && p.URL != p.PDFURL {
			webButton = fmt.Sprintf(`<a href="%s" target="_blank" style="display: inline-block; text-decoration: none; font-size: 13px; font-weight: 600; color: #334155; background-color: #f1f5f9; padding: 8px 16px; border-radius: 6px; border: 1px solid #cbd5e1; transition: background-color 0.2s;">Web Page</a>`, safeHref(p.URL))
		}
		pdfButton := fmt.Sprintf(`<a href="%s" target="_blank" style="display: inline-block; text-decoration: none; font-size: 13px; font-weight: 700; color: #ffffff; background-color: #dc2626; padding: 8px 20px; border-radius: 6px; box-shadow: 0 1px 2px rgba(220, 38, 38, 0.2); transition: background-color 0.2s;">PDF</a>`, safeHref(p.PDFURL))

		block := fmt.Sprintf(`
		<div style="background-color: #ffffff; border-radius: 12px; box-shadow: 0 4px 6px rgba(0, 0, 0, 0.05); margin-bottom: 24px; padding: 24px; border: 1px solid #eaeaea; font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;">
			<h2 style="font-size: 19px; font-weight: 700; color: #1a1a1a; margin-top: 0; margin-bottom: 8px; line-height: 1.4;">%s</h2>
			<div style="font-size: 14px; color: #666666; margin-bottom: 14px; line-height: 1.5;">
				<span style="font-weight: 500;">%s</span>
				%s
			</div>
			
			<div style="display: inline-flex; align-items: center; margin-bottom: 16px; font-size: 13px; color: #166534; font-weight: 600; background-color: #f0fdf4; border: 1px solid #bbf7d0; padding: 5px 12px; border-radius: 16px;">
				<span style="margin-right: 6px;">Relevance Score: %.1f</span>
				%s
			</div>

			%s
			%s

			<div style="margin-top: 16px; display: flex; gap: 8px; align-items: center;">
				%s
				%s
			</div>
		</div>`, formattedTitle, html.EscapeString(authors), affBlock, p.Score, stars, tldrBlock, abstractBlock, pdfButton, webButton)

		cardBlocks = append(cardBlocks, block)
	}

	content := strings.Join(cardBlocks, "\n")
	today := time.Now().Format("Monday, Jan 2, 2006")

	emailTemplate := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Daily arXiv Digest</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f6f8; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;">
	<div style="max-width: 680px; margin: 0 auto; padding: 20px;">
		<!-- Header -->
		<div style="background: linear-gradient(135deg, #1e293b, #0f172a); border-radius: 12px; padding: 32px 24px; margin-bottom: 24px; text-align: center; color: #ffffff; box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);">
			<h1 style="font-size: 28px; font-weight: 800; margin: 0 0 8px 0; letter-spacing: -0.5px;">Daily arXiv Digest</h1>
			<p style="font-size: 16px; color: #94a3b8; margin: 0; font-weight: 500;">%s</p>
		</div>

		<!-- Content -->
		<div>
			%s
		</div>

		<!-- Footer -->
		<div style="text-align: center; padding: 32px 0 16px 0; font-size: 12px; color: #94a3b8; line-height: 1.5; border-top: 1px solid #e2e8f0; margin-top: 32px;">
			<p style="margin: 0 0 8px 0;">This email was automatically generated and sent to you.</p>
			<p style="margin: 0;">To unsubscribe, remove your email from your GitHub repository config settings.</p>
		</div>
	</div>
</body>
</html>`, today, content)

	return emailTemplate
}

func renderEmptyEmail() string {
	today := time.Now().Format("Monday, Jan 2, 2006")
	return fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Daily arXiv Digest</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f6f8; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;">
	<div style="max-width: 680px; margin: 0 auto; padding: 20px;">
		<!-- Header -->
		<div style="background: linear-gradient(135deg, #1e293b, #0f172a); border-radius: 12px; padding: 32px 24px; margin-bottom: 24px; text-align: center; color: #ffffff; box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);">
			<h1 style="font-size: 28px; font-weight: 800; margin: 0 0 8px 0; letter-spacing: -0.5px;">Daily arXiv Digest</h1>
			<p style="font-size: 16px; color: #94a3b8; margin: 0; font-weight: 500;">%s</p>
		</div>

		<!-- Content -->
		<div style="background-color: #ffffff; border-radius: 12px; box-shadow: 0 4px 6px rgba(0, 0, 0, 0.05); padding: 40px 24px; border: 1px solid #eaeaea; text-align: center;">
			<div style="font-size: 48px; margin-bottom: 16px;">☕</div>
			<h2 style="font-size: 22px; font-weight: 700; color: #1a1a1a; margin: 0 0 12px 0;">No Papers Found Today</h2>
			<p style="font-size: 15px; color: #666666; margin: 0; line-height: 1.6;">Take a break! There are no matching papers matching your interests in today's updates.</p>
		</div>

		<!-- Footer -->
		<div style="text-align: center; padding: 32px 0 16px 0; font-size: 12px; color: #94a3b8; line-height: 1.5; border-top: 1px solid #e2e8f0; margin-top: 32px;">
			<p style="margin: 0 0 8px 0;">This email was automatically generated and sent to you.</p>
			<p style="margin: 0;">To unsubscribe, remove your email from your GitHub repository config settings.</p>
		</div>
	</div>
</body>
</html>`, today)
}

func getStarsHTML(score float64) string {
	low := 6.0
	high := 8.0
	if score <= low {
		return ""
	}

	fullStar := `<span style="color: #ffb300; margin-right: 1px;">⭐</span>`
	halfStar := `<span style="color: #ffb300; opacity: 0.5; margin-right: 1px;">⭐</span>`

	if score >= high {
		return strings.Repeat(fullStar, 5)
	}

	interval := (high - low) / 10.0
	starCount := int((score - low) / interval)
	fullCount := starCount / 2
	halfCount := starCount % 2

	var stars strings.Builder
	stars.WriteString(`<div style="display: inline-flex; align-items: center; margin-left: 6px;">`)
	for i := 0; i < fullCount; i++ {
		stars.WriteString(fullStar)
	}
	for i := 0; i < halfCount; i++ {
		stars.WriteString(halfStar)
	}
	stars.WriteString(`</div>`)
	return stars.String()
}

// SendEmail sends the HTML email using SMTP with TLS -> SSL -> plain text fallbacks.
func SendEmail(cfg config.EmailConfig, html string) error {
	today := time.Now().Format("2006/01/02")
	subject := fmt.Sprintf("Daily arXiv %s", today)

	// Format From/To fields safely
	mimeHeaders := fmt.Sprintf("MIME-Version: 1.0\r\n"+
		"From: GitHub Action <%s>\r\n"+
		"To: You <%s>\r\n"+
		"Subject: %s\r\n"+
		"Content-Type: text/html; charset=UTF-8\r\n\r\n", cfg.Sender, cfg.Receiver, subject)

	msg := []byte(mimeHeaders + html)
	addr := fmt.Sprintf("%s:%d", cfg.SMTPServer, cfg.SMTPPort)
	auth := smtp.PlainAuth("", cfg.SMTPUser, cfg.SenderPassword, cfg.SMTPServer)

	// 1. SSL implicitly on port 465
	if cfg.SMTPPort == 465 {
		tlsConfig := &tls.Config{
			ServerName: cfg.SMTPServer,
		}
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("SSL dial failed on port 465: %w", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, cfg.SMTPServer)
		if err != nil {
			return fmt.Errorf("failed to create SMTP client: %w", err)
		}
		defer client.Close()

		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP SSL auth failed: %w", err)
		}

		return sendMailThroughClient(client, cfg.Sender, cfg.Receiver, msg)
	}

	// 2. Starttls/Plain fallback for ports like 587 or 25
	client, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("SMTP plain dial failed: %w", err)
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsConfig := &tls.Config{
			ServerName: cfg.SMTPServer,
		}
		if err = client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("STARTTLS failed: %w", err)
		}
	} else if cfg.SMTPPort != 25 {
		return fmt.Errorf("SMTP server %s does not support STARTTLS; refusing to transmit credentials in cleartext", cfg.SMTPServer)
	}

	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP authentication failed: %w", err)
	}

	return sendMailThroughClient(client, cfg.Sender, cfg.Receiver, msg)
}

func sendMailThroughClient(client *smtp.Client, sender, receiver string, msg []byte) error {
	if err := client.Mail(sender); err != nil {
		return fmt.Errorf("failed to set SMTP MAIL FROM: %w", err)
	}
	if err := client.Rcpt(receiver); err != nil {
		return fmt.Errorf("failed to set SMTP RCPT TO: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to open SMTP DATA write channel: %w", err)
	}
	defer w.Close()

	if _, err = w.Write(msg); err != nil {
		return fmt.Errorf("failed to write message body: %w", err)
	}

	return client.Quit()
}
