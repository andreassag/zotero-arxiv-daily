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
