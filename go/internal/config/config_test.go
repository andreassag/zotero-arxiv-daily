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

package config

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleBaseYAML = `
zotero:
  user_id: "123456"
  api_key: "base-key"
  include_path: ["2026/survey/**"]
  ignore_path: null

source:
  arxiv:
    category: ["cs.AI"]
    include_cross_list: false

email:
  sender: "sender@example.com"
  receiver: "receiver@example.com"
  smtp_server: "smtp.example.com"
  smtp_port: 587
  sender_password: "password123"

llm:
  api:
    key: "openai-key"
    base_url: "https://api.openai.com/v1"
  generation_kwargs:
    model: "gpt-4o-mini"
    max_tokens: 150
  language: "English"

reranker:
  api:
    key: "reranker-key"
    base_url: "https://api.openai.com/v1"
    model: "text-embedding-3-large"
    batch_size: 64

executor:
  debug: false
  send_empty: false
  max_paper_num: 100
  source: ["arxiv"]
`

const sampleCustomYAML = `
zotero:
  api_key: "${oc.env:TEST_ZOTERO_KEY,custom-key}"
email:
  smtp_port: ${oc.env:TEST_SMTP_PORT,465}
`

func TestExpandEnv(t *testing.T) {
	t.Setenv("TEST_VAR", "expanded-value")

	// Test standard env replacement
	expanded := ExpandEnv("key: ${TEST_VAR}")
	if expanded != "key: expanded-value" {
		t.Errorf("expected 'key: expanded-value', got '%s'", expanded)
	}

	// Test oc.env syntax
	expanded2 := ExpandEnv("key: ${oc.env:TEST_VAR}")
	if expanded2 != "key: expanded-value" {
		t.Errorf("expected 'key: expanded-value', got '%s'", expanded2)
	}

	// Test fallback default syntax (when env is missing)
	expanded3 := ExpandEnv("key: ${oc.env:MISSING_VAR,default-val}")
	if expanded3 != "key: default-val" {
		t.Errorf("expected 'key: default-val', got '%s'", expanded3)
	}
}

func TestLoadConfigFromDir(t *testing.T) {
	tmpDir := t.TempDir()

	err := os.WriteFile(filepath.Join(tmpDir, "base.yaml"), []byte(sampleBaseYAML), 0644)
	if err != nil {
		t.Fatalf("failed to write base.yaml: %v", err)
	}

	err = os.WriteFile(filepath.Join(tmpDir, "custom.yaml"), []byte(sampleCustomYAML), 0644)
	if err != nil {
		t.Fatalf("failed to write custom.yaml: %v", err)
	}

	cfg, err := LoadConfigFromDir(tmpDir)
	if err != nil {
		t.Fatalf("expected no error loading config, got %v", err)
	}

	// Check if base parameters parsed correctly
	if cfg.Zotero.UserID != "123456" {
		t.Errorf("expected UserID '123456', got '%s'", cfg.Zotero.UserID)
	}

	// Check if custom.yaml merged correctly and default values expanded
	if cfg.Zotero.APIKey != "custom-key" {
		t.Errorf("expected APIKey 'custom-key', got '%s'", cfg.Zotero.APIKey)
	}

	if cfg.Email.SMTPPort != 465 {
		t.Errorf("expected SMTPPort 465, got %d", cfg.Email.SMTPPort)
	}

	// Check if missing required fields raises validation error
	invalidYAML := `
zotero:
  user_id: "???"
`
	tmpDir2 := t.TempDir()
	err = os.WriteFile(filepath.Join(tmpDir2, "base.yaml"), []byte(invalidYAML), 0644)
	if err != nil {
		t.Fatalf("failed to write invalid base.yaml: %v", err)
	}

	_, err = LoadConfigFromDir(tmpDir2)
	if err == nil {
		t.Error("expected validation error due to '???' in user_id, got nil")
	}
}

func TestExpandEnv_NestedAndDecode(t *testing.T) {
	t.Setenv("PORT_VAR", "587")
	t.Setenv("DEBUG_VAR", "true")
	t.Setenv("FALLBACK_VAL", "secondary")

	// Test oc.decode with env var
	expanded1 := ExpandEnv("port: ${oc.decode:${oc.env:PORT_VAR}}")
	if expanded1 != "port: 587" {
		t.Errorf("expected 'port: 587', got '%s'", expanded1)
	}

	// Test oc.decode with boolean
	expanded2 := ExpandEnv("debug: ${oc.decode:${oc.env:DEBUG_VAR}}")
	if expanded2 != "debug: true" {
		t.Errorf("expected 'debug: true', got '%s'", expanded2)
	}

	// Test oc.decode with default
	expanded3 := ExpandEnv("port: ${oc.decode:${oc.env:NONEXISTENT_PORT,465}}")
	if expanded3 != "port: 465" {
		t.Errorf("expected 'port: 465', got '%s'", expanded3)
	}

	// Test nested fallback ${PRIMARY,${SECONDARY}}
	expanded4 := ExpandEnv("val: ${PRIMARY_UNSET,${FALLBACK_VAL}}")
	if expanded4 != "val: secondary" {
		t.Errorf("expected 'val: secondary', got '%s'", expanded4)
	}
}

func TestRateLimitConfigParsing(t *testing.T) {
	rateLimitYAML := `
zotero:
  user_id: "123"
  api_key: "key"
llm:
  rate_limit:
    rpm: 15
    tpm: 250000
    rpd: 500
reranker:
  rate_limit:
    rpm: 100
    tpm: 30000
    rpd: 1000
`
	tmpDir := t.TempDir()
	err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(rateLimitYAML), 0644)
	if err != nil {
		t.Fatalf("failed to write config.yaml: %v", err)
	}

	cfg, err := LoadConfigFromDir(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.LLM.RateLimit.RPM != 15 || cfg.LLM.RateLimit.TPM != 250000 || cfg.LLM.RateLimit.RPD != 500 {
		t.Errorf("unexpected LLM rate limits: %+v", cfg.LLM.RateLimit)
	}
	if cfg.Reranker.RateLimit.RPM != 100 || cfg.Reranker.RateLimit.TPM != 30000 || cfg.Reranker.RateLimit.RPD != 1000 {
		t.Errorf("unexpected Reranker rate limits: %+v", cfg.Reranker.RateLimit)
	}
}
