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
	"strings"
	"testing"
	"time"
)

func TestEstimateTokens(t *testing.T) {
	if got := EstimateTokens("abcd"); got != 1 {
		t.Errorf("expected 1 token, got %d", got)
	}
	if got := EstimateTokens(strings.Repeat("a", 40)); got != 10 {
		t.Errorf("expected 10 tokens, got %d", got)
	}
	if got := EstimateTokens(""); got != 1 {
		t.Errorf("expected 1 token for empty string, got %d", got)
	}

	slice := []string{strings.Repeat("a", 20), strings.Repeat("b", 20)}
	if got := EstimateTokensSlice(slice); got != 10 {
		t.Errorf("expected 10 tokens for slice, got %d", got)
	}
}

func TestRPDExceeded(t *testing.T) {
	limiter := New("TestRPD", Config{
		RPD: 2,
	})

	ctx := context.Background()
	if err := limiter.Acquire(ctx, 10); err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	if err := limiter.Acquire(ctx, 10); err != nil {
		t.Fatalf("second acquire failed: %v", err)
	}

	// Third request exceeds RPD=2
	err := limiter.Acquire(ctx, 10)
	if err == nil {
		t.Fatal("expected RPD exceeded error, got nil")
	}
	if !strings.Contains(err.Error(), "daily request limit") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRPMWait(t *testing.T) {
	limiter := New("TestRPM", Config{
		RPM: 2,
	})

	now := time.Now()
	// Simulate 2 requests recorded 59.95s ago
	limiter.minuteRequests = []time.Time{
		now.Add(-59950 * time.Millisecond),
		now.Add(-59950 * time.Millisecond),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	if err := limiter.Acquire(ctx, 1); err != nil {
		t.Fatalf("acquire failed: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 40*time.Millisecond {
		t.Errorf("expected to wait for RPM window, but acquired in %v", elapsed)
	}
}

func TestContextCancel(t *testing.T) {
	limiter := New("TestCancel", Config{
		RPM: 1,
	})

	// Record a request just now
	limiter.minuteRequests = []time.Time{time.Now()}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := limiter.Acquire(ctx, 1)
	if err == nil {
		t.Fatal("expected context deadline error, got nil")
	}
}

func TestOversizedTokens(t *testing.T) {
	limiter := New("TestOversized", Config{
		TPM: 100,
	})

	err := limiter.Acquire(context.Background(), 200)
	if err == nil {
		t.Fatal("expected error for request exceeding TPM, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds maximum configured TPM") {
		t.Errorf("unexpected error: %v", err)
	}
}
