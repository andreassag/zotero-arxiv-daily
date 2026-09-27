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

package ratelimit

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Config specifies rate limits for an API client.
type Config struct {
	RPM int // Requests per minute
	TPM int // Tokens per minute
	RPD int // Requests per day
}

type tokenEntry struct {
	timestamp time.Time
	tokens    int
}

// Limiter manages sliding-window rate limits (RPM, TPM, RPD) to prevent API lockout.
type Limiter struct {
	name string
	cfg  Config
	mu   sync.Mutex

	minuteRequests []time.Time
	minuteTokens   []tokenEntry
	dayRequests    []time.Time
}

// New creates a new Limiter with the specified name and limits.
func New(name string, cfg Config) *Limiter {
	return &Limiter{
		name: name,
		cfg:  cfg,
	}
}

// EstimateTokens calculates an approximate token count (~4 characters per token).
func EstimateTokens(text string) int {
	tokens := len(text) / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}

// EstimateTokensSlice estimates total tokens for a slice of strings.
func EstimateTokensSlice(texts []string) int {
	total := 0
	for _, t := range texts {
		total += EstimateTokens(t)
	}
	if total < 1 {
		return 1
	}
	return total
}

// Acquire blocks until rate limits allow sending a request with estimatedTokens,
// or returns an error if RPD limit is exceeded or context is canceled.
func (l *Limiter) Acquire(ctx context.Context, estimatedTokens int) error {
	if estimatedTokens < 1 {
		estimatedTokens = 1
	}

	for {
		l.mu.Lock()
		now := time.Now()

		// 1. Enforce Requests Per Day (RPD) - 24 hour sliding window
		if l.cfg.RPD > 0 {
			dayThreshold := now.Add(-24 * time.Hour)
			validDay := l.dayRequests[:0]
			for _, t := range l.dayRequests {
				if t.After(dayThreshold) {
					validDay = append(validDay, t)
				}
			}
			l.dayRequests = validDay

			if len(l.dayRequests) >= l.cfg.RPD {
				l.mu.Unlock()
				return fmt.Errorf("[%s] daily request limit (%d RPD) exceeded, halting to prevent system lockout", l.name, l.cfg.RPD)
			}
		}

		// 2. Prune 1-minute sliding window (60 seconds)
		minuteThreshold := now.Add(-60 * time.Second)

		validMinReqs := l.minuteRequests[:0]
		for _, t := range l.minuteRequests {
			if t.After(minuteThreshold) {
				validMinReqs = append(validMinReqs, t)
			}
		}
		l.minuteRequests = validMinReqs

		validMinTokens := l.minuteTokens[:0]
		currentMinuteTokens := 0
		for _, te := range l.minuteTokens {
			if te.timestamp.After(minuteThreshold) {
				validMinTokens = append(validMinTokens, te)
				currentMinuteTokens += te.tokens
			}
		}
		l.minuteTokens = validMinTokens

		// 3. Calculate wait times for RPM and TPM
		var rpmWait time.Duration
		if l.cfg.RPM > 0 && len(l.minuteRequests) >= l.cfg.RPM {
			oldestReq := l.minuteRequests[0]
			wait := 60*time.Second - now.Sub(oldestReq) + 100*time.Millisecond
			if wait > rpmWait {
				rpmWait = wait
			}
		}

		var tpmWait time.Duration
		if l.cfg.TPM > 0 && (currentMinuteTokens+estimatedTokens) > l.cfg.TPM {
			if len(l.minuteTokens) > 0 {
				oldestToken := l.minuteTokens[0].timestamp
				wait := 60*time.Second - now.Sub(oldestToken) + 100*time.Millisecond
				if wait > tpmWait {
					tpmWait = wait
				}
			}
		}

		waitTime := rpmWait
		if tpmWait > waitTime {
			waitTime = tpmWait
		}

		if waitTime <= 0 {
			// Request allowed; record and release
			l.minuteRequests = append(l.minuteRequests, now)
			l.minuteTokens = append(l.minuteTokens, tokenEntry{timestamp: now, tokens: estimatedTokens})
			if l.cfg.RPD > 0 {
				l.dayRequests = append(l.dayRequests, now)
			}
			l.mu.Unlock()
			return nil
		}

		l.mu.Unlock()

		slog.Info(fmt.Sprintf("[%s] Rate limit approaching (RPM: %d/%d, TPM: %d/%d). Pausing for %v to prevent lockout...",
			l.name, len(validMinReqs), l.cfg.RPM, currentMinuteTokens+estimatedTokens, l.cfg.TPM, waitTime.Round(time.Millisecond)))

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
		}
	}
}
