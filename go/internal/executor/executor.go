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
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/exTerEX/zotero-arxiv-daily/go/internal/config"
	"github.com/exTerEX/zotero-arxiv-daily/go/internal/email"
	"github.com/exTerEX/zotero-arxiv-daily/go/internal/llm"
	"github.com/exTerEX/zotero-arxiv-daily/go/internal/model"
	"github.com/exTerEX/zotero-arxiv-daily/go/internal/ratelimit"
	"github.com/exTerEX/zotero-arxiv-daily/go/internal/reranker"
	"github.com/exTerEX/zotero-arxiv-daily/go/internal/retriever"
	"github.com/exTerEX/zotero-arxiv-daily/go/zotero"
)

type Executor struct {
	cfg         *config.Config
	httpClient  *http.Client
	llmClient   *llm.Client
	zotClient   *zotero.Client
	reranker    *reranker.Reranker
	includeRegs []*regexp.Regexp
	ignoreRegs  []*regexp.Regexp
}

func New(cfg *config.Config) (*Executor, error) {
	var llmOpts []llm.Option
	if cfg.LLM.RateLimit.RPM > 0 || cfg.LLM.RateLimit.TPM > 0 || cfg.LLM.RateLimit.RPD > 0 {
		llmLimiter := ratelimit.New("LLM", ratelimit.Config{
			RPM: cfg.LLM.RateLimit.RPM,
			TPM: cfg.LLM.RateLimit.TPM,
			RPD: cfg.LLM.RateLimit.RPD,
		})
		llmOpts = append(llmOpts, llm.WithRateLimiter(llmLimiter))
	}
	llmClient := llm.NewClient(cfg.LLM.API.Key, cfg.LLM.API.BaseURL, llmOpts...)

	// Reranker client
	rerankKey := cfg.Reranker.API.Key
	if rerankKey == "" {
		rerankKey = cfg.LLM.API.Key
	}
	rerankBase := cfg.Reranker.API.BaseURL
	if rerankBase == "" {
		rerankBase = cfg.LLM.API.BaseURL
	}

	var rerankOpts []llm.Option
	if cfg.Reranker.RateLimit.RPM > 0 || cfg.Reranker.RateLimit.TPM > 0 || cfg.Reranker.RateLimit.RPD > 0 {
		rerankLimiter := ratelimit.New("Reranker", ratelimit.Config{
			RPM: cfg.Reranker.RateLimit.RPM,
			TPM: cfg.Reranker.RateLimit.TPM,
			RPD: cfg.Reranker.RateLimit.RPD,
		})
		rerankOpts = append(rerankOpts, llm.WithRateLimiter(rerankLimiter))
	}
	rerankLLM := llm.NewClient(rerankKey, rerankBase, rerankOpts...)

	zotClient := zotero.NewClient(cfg.Zotero.UserID, cfg.Zotero.APIKey)
	ranker := reranker.New(rerankLLM, cfg.Reranker.API.Model, cfg.Reranker.API.BatchSize)

	// Compile glob pattern regexes
	includeRegs, err := compileGlobs(cfg.Zotero.IncludePath)
	if err != nil {
		return nil, fmt.Errorf("failed to compile include path patterns: %w", err)
	}

	ignoreRegs, err := compileGlobs(cfg.Zotero.IgnorePath)
	if err != nil {
		return nil, fmt.Errorf("failed to compile ignore path patterns: %w", err)
	}

	return &Executor{
		cfg:         cfg,
		httpClient:  &http.Client{Timeout: 15 * time.Second},
		llmClient:   llmClient,
		zotClient:   zotClient,
		reranker:    ranker,
		includeRegs: includeRegs,
		ignoreRegs:  ignoreRegs,
	}, nil
}

func compileGlobs(patterns []string) ([]*regexp.Regexp, error) {
	var regs []*regexp.Regexp
	for _, pat := range patterns {
		if pat == "" {
			continue
		}
		// Convert standard glob to regex
		escaped := regexp.QuoteMeta(pat)
		r := strings.ReplaceAll(escaped, "\\*\\*", ".*")
		r = strings.ReplaceAll(r, "\\*", "[^/]*")
		r = strings.ReplaceAll(r, "\\?", "[^/]")
		reg, err := regexp.Compile("^" + r + "$")
		if err != nil {
			return nil, fmt.Errorf("invalid pattern '%s': %w", pat, err)
		}
		regs = append(regs, reg)
	}
	return regs, nil
}

func (e *Executor) Run(ctx context.Context) error {
	slog.Info("Starting workflow run")

	// 1. Fetch Zotero Corpus
	rawCorpus, err := e.zotClient.FetchCorpus(ctx)
	if err != nil {
		return fmt.Errorf("zotero fetch failed: %w", err)
	}

	// Map public zotero.CorpusPaper to internal model.CorpusPaper
	var corpus []model.CorpusPaper
	for _, p := range rawCorpus {
		corpus = append(corpus, model.CorpusPaper{
			Title:     p.Title,
			Abstract:  p.Abstract,
			AddedDate: p.AddedDate,
			Paths:     p.Paths,
		})
	}

	// 2. Filter Zotero Corpus
	corpus = e.filterCorpus(corpus)
	slog.Info("Filtered Zotero corpus", "fetched_count", len(rawCorpus), "filtered_count", len(corpus))

	if len(corpus) == 0 {
		slog.Warn("No Zotero papers found matching collection criteria. Cannot compute relevance scores.")
		if e.cfg.Executor.SendEmpty {
			return e.handleEmailSending(ctx, nil)
		}
		return nil
	}

	// 3. Retrieve papers from all sources in parallel
	var allPapers []model.Paper
	var papersMu sync.Mutex
	var wg sync.WaitGroup
	errs := make(chan error, len(e.cfg.Executor.Sources))

	for _, source := range e.cfg.Executor.Sources {
		wg.Add(1)
		go func(src string) {
			defer wg.Done()
			slog.Info("Starting paper retrieval", "source", src)

			ret, err := retriever.Get(src, e.cfg)
			if err != nil {
				errs <- fmt.Errorf("failed to get retriever %s: %w", src, err)
				return
			}

			papers, err := ret.RetrievePapers(ctx)
			if err != nil {
				errs <- fmt.Errorf("retrieval failed for %s: %w", src, err)
				return
			}

			papersMu.Lock()
			allPapers = append(allPapers, papers...)
			papersMu.Unlock()

			slog.Info("Completed paper retrieval", "source", src, "retrieved_count", len(papers))
		}(source)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			return err
		}
	}

	slog.Info("Retrieved all papers", "total_count", len(allPapers))

	// 4. Rerank papers
	var rankedPapers []model.Paper
	if len(allPapers) > 0 {
		slog.Info("Reranking papers against Zotero library...")
		rankedPapers, err = e.reranker.Rerank(ctx, allPapers, corpus)
		if err != nil {
			return fmt.Errorf("reranking failed: %w", err)
		}

		// Truncate to maximum papers limit
		maxNum := e.cfg.Executor.MaxPaperNum
		if maxNum > 0 && len(rankedPapers) > maxNum {
			rankedPapers = rankedPapers[:maxNum]
		}

		// 5. Generate TL;DRs and extract affiliations
		slog.Info("Generating TL;DRs and affiliations...")
		for i := range rankedPapers {
			tldr, err := e.llmClient.GenerateTLDR(ctx, rankedPapers[i].Title, rankedPapers[i].Abstract, e.cfg.LLM.Language, e.cfg.LLM.GenerationKwargs.Model, e.cfg.LLM.GenerationKwargs.MaxTokens)
			if err != nil {
				slog.Warn("Failed to generate TL;DR, falling back to abstract", "paper", rankedPapers[i].Title, "error", err)
				rankedPapers[i].TLDR = rankedPapers[i].Abstract
			} else {
				rankedPapers[i].TLDR = tldr
			}

			affs := ExtractAffiliations(ctx, e.httpClient, e.llmClient, &rankedPapers[i], e.cfg.LLM.GenerationKwargs.Model)
			rankedPapers[i].Affiliations = affs
		}
	}

	// 6. Handle email generation & delivery
	return e.handleEmailSending(ctx, rankedPapers)
}

func (e *Executor) filterCorpus(corpus []model.CorpusPaper) []model.CorpusPaper {
	if len(e.includeRegs) == 0 && len(e.ignoreRegs) == 0 {
		return corpus
	}

	var filtered []model.CorpusPaper
	for _, p := range corpus {
		matchInclude := len(e.includeRegs) == 0
		if len(e.includeRegs) > 0 {
			for _, path := range p.Paths {
				for _, reg := range e.includeRegs {
					if reg.MatchString(path) {
						matchInclude = true
						break
					}
				}
				if matchInclude {
					break
				}
			}
		}

		matchIgnore := false
		if len(e.ignoreRegs) > 0 {
			for _, path := range p.Paths {
				for _, reg := range e.ignoreRegs {
					if reg.MatchString(path) {
						matchIgnore = true
						break
					}
				}
				if matchIgnore {
					break
				}
			}
		}

		if matchInclude && !matchIgnore {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func (e *Executor) handleEmailSending(ctx context.Context, papers []model.Paper) error {
	if len(papers) == 0 && !e.cfg.Executor.SendEmpty {
		slog.Info("No new papers to send, skipping email.")
		return nil
	}

	emailHTML := email.RenderEmail(papers)

	noEmail := strings.ToLower(os.Getenv("NO_EMAIL"))
	if noEmail == "true" || noEmail == "1" || noEmail == "yes" {
		slog.Info("NO_EMAIL environment variable set. Skipping email dispatch.")

		rawPath := os.Getenv("SAVE_EMAIL_PATH")
		if rawPath != "" {
			savePath := filepath.Clean(rawPath)
			if err := os.WriteFile(savePath, []byte(emailHTML), 0600); err != nil {
				slog.Error("Failed to save email output to file", "path", savePath, "error", err)
			} else {
				slog.Info("Saved email content to local file", "path", savePath)
			}
		}
		return nil
	}

	slog.Info("Dispatching email digest...")
	if err := email.SendEmail(e.cfg.Email, emailHTML); err != nil {
		return fmt.Errorf("email delivery failed: %w", err)
	}

	slog.Info("Email sent successfully!")
	return nil
}
