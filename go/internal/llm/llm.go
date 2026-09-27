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

package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/exTerEX/zotero-arxiv-daily/go/internal/ratelimit"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type Client struct {
	client  *openai.Client
	limiter *ratelimit.Limiter
}

// Option configures an LLM Client.
type Option func(*Client)

// WithRateLimiter attaches a RateLimiter to the Client.
func WithRateLimiter(limiter *ratelimit.Limiter) Option {
	return func(c *Client) {
		c.limiter = limiter
	}
}

// NewClient creates a new OpenAI Go SDK wrapper.
func NewClient(apiKey, baseURL string, opts ...Option) *Client {
	var reqOpts []option.RequestOption
	if apiKey != "" {
		reqOpts = append(reqOpts, option.WithAPIKey(apiKey))
	}
	if baseURL != "" {
		reqOpts = append(reqOpts, option.WithBaseURL(baseURL))
	}
	client := openai.NewClient(reqOpts...)
	c := &Client{
		client: &client,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// GenerateTLDR uses ChatCompletions to generate a 2-3 sentence summary.
func (c *Client) GenerateTLDR(ctx context.Context, title, abstract, language, model string, maxTokens int) (string, error) {
	prompt := fmt.Sprintf("Summarize the paper in 2-3 concise sentences in %s, focusing on what it does and its main results or conclusion.\n\n", language)
	if title != "" {
		prompt += fmt.Sprintf("Title: %s\n\n", title)
	}
	if abstract != "" {
		prompt += fmt.Sprintf("Abstract: %s\n\n", abstract)
	}
	prompt = strings.TrimSpace(prompt)

	// Simple character limit truncation to keep prompt sizes safe
	if len(prompt) > 8000 {
		prompt = prompt[:8000]
	}

	systemContent := fmt.Sprintf("You are an assistant that summarizes scientific papers clearly and concisely. Write 2-3 sentences describing the paper's purpose and main result or conclusion in %s.", language)

	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemContent),
			openai.UserMessage(prompt),
		},
		Model: model,
	}
	if maxTokens > 0 {
		params.MaxCompletionTokens = openai.Int(int64(maxTokens))
	}

	if c.limiter != nil {
		estTokens := ratelimit.EstimateTokens(systemContent) + ratelimit.EstimateTokens(prompt)
		if err := c.limiter.Acquire(ctx, estTokens); err != nil {
			return "", err
		}
	}

	resp, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response choices returned from LLM")
	}

	return resp.Choices[0].Message.Content, nil
}

var jsonArrayRegex = regexp.MustCompile(`\[.*?\]`)

// ExtractAffiliations extracts the affiliations of authors from fullText or title/authors/abstract.
func (c *Client) ExtractAffiliations(ctx context.Context, title string, authors []string, abstract, fullText, model string) ([]string, error) {
	content := fullText
	if strings.TrimSpace(content) == "" {
		var parts []string
		if title != "" {
			parts = append(parts, "Title: "+title)
		}
		if len(authors) > 0 {
			parts = append(parts, "Authors: "+strings.Join(authors, ", "))
		}
		if abstract != "" {
			parts = append(parts, "Abstract: "+abstract)
		}
		content = strings.Join(parts, "\n\n")
	}
	if strings.TrimSpace(content) == "" {
		return nil, nil
	}

	prompt := fmt.Sprintf("Given the following scientific paper details, extract the affiliations or institutions of the authors in a JSON list format [\"Institution 1\", \"Institution 2\"]. If an affiliation has multi-level departments, extract the top-level institution or university name only. If no affiliation can be determined, return an empty list []. Return only the JSON list:\n\n%s", content)
	if len(prompt) > 8000 {
		prompt = prompt[:8000]
	}

	systemContent := "You are an assistant who extracts affiliations of authors from a scientific paper. Return a JSON list of unique institutions, like [\"Stanford University\", \"Tsinghua University\"]. If no affiliation is found, return []. Only return the JSON list, with no surrounding explanations."

	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemContent),
			openai.UserMessage(prompt),
		},
		Model: model,
	}

	if c.limiter != nil {
		estTokens := ratelimit.EstimateTokens(systemContent) + ratelimit.EstimateTokens(prompt)
		if err := c.limiter.Acquire(ctx, estTokens); err != nil {
			return nil, err
		}
	}

	resp, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response choices returned from LLM")
	}

	content = resp.Choices[0].Message.Content
	match := jsonArrayRegex.FindString(strings.ReplaceAll(content, "\n", " "))
	if match == "" {
		return nil, fmt.Errorf("failed to locate JSON list in LLM response: %s", content)
	}

	var affiliations []string
	if err := json.Unmarshal([]byte(match), &affiliations); err != nil {
		return nil, fmt.Errorf("failed to parse JSON list from LLM response: %w", err)
	}

	seen := make(map[string]bool)
	var deduped []string
	for _, aff := range affiliations {
		cleaned := strings.TrimSpace(aff)
		if cleaned != "" && !seen[cleaned] {
			seen[cleaned] = true
			deduped = append(deduped, cleaned)
		}
	}

	return deduped, nil
}

// GetEmbeddings retrieves vector embeddings for a list of texts in batches.
func (c *Client) GetEmbeddings(ctx context.Context, texts []string, model string, batchSize int) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if batchSize <= 0 {
		batchSize = 64
	}

	var allEmbeddings [][]float64
	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]

		params := openai.EmbeddingNewParams{
			Input: openai.EmbeddingNewParamsInputUnion{
				OfArrayOfStrings: batch,
			},
			Model: model,
		}

		if c.limiter != nil {
			estTokens := ratelimit.EstimateTokensSlice(batch)
			if err := c.limiter.Acquire(ctx, estTokens); err != nil {
				return nil, err
			}
		}

		resp, err := c.client.Embeddings.New(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("embedding creation failed: %w", err)
		}

		batchEmbeds := make([][]float64, len(batch))
		for _, item := range resp.Data {
			if item.Index >= 0 && item.Index < int64(len(batch)) {
				batchEmbeds[item.Index] = item.Embedding
			}
		}
		allEmbeddings = append(allEmbeddings, batchEmbeds...)
	}

	return allEmbeddings, nil
}
