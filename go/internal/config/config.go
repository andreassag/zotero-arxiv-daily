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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type ZoteroConfig struct {
	UserID      string   `yaml:"user_id"`
	APIKey      string   `yaml:"api_key"`
	IncludePath []string `yaml:"include_path"`
	IgnorePath  []string `yaml:"ignore_path"`
}

type ArxivConfig struct {
	Category         []string `yaml:"category"`
	IncludeCrossList bool     `yaml:"include_cross_list"`
}

type BiorxivConfig struct {
	Category []string `yaml:"category"`
}

type MedrxivConfig struct {
	Category []string `yaml:"category"`
}

type SourceConfig struct {
	Arxiv   ArxivConfig   `yaml:"arxiv"`
	Biorxiv BiorxivConfig `yaml:"biorxiv"`
	Medrxiv MedrxivConfig `yaml:"medrxiv"`
}

type EmailConfig struct {
	Sender         string `yaml:"sender"`
	Receiver       string `yaml:"receiver"`
	SMTPServer     string `yaml:"smtp_server"`
	SMTPPort       int    `yaml:"smtp_port"`
	SenderPassword string `yaml:"sender_password"`
	SMTPUser       string `yaml:"smtp_user"`
	SMTPUsername   string `yaml:"smtp_username"`
}

type APIConfig struct {
	Key     string `yaml:"key"`
	BaseURL string `yaml:"base_url"`
}

type LLMGenerationKwargs struct {
	Model     string `yaml:"model"`
	MaxTokens int    `yaml:"max_tokens"`
}

type RateLimitConfig struct {
	RPM int `yaml:"rpm"`
	TPM int `yaml:"tpm"`
	RPD int `yaml:"rpd"`
}

type LLMConfig struct {
	API              APIConfig           `yaml:"api"`
	GenerationKwargs LLMGenerationKwargs `yaml:"generation_kwargs"`
	Language         string              `yaml:"language"`
	RateLimit        RateLimitConfig     `yaml:"rate_limit"`
}

type RerankerAPIConfig struct {
	Key       string `yaml:"key"`
	BaseURL   string `yaml:"base_url"`
	Model     string `yaml:"model"`
	BatchSize int    `yaml:"batch_size"`
}

type RerankerConfig struct {
	API       RerankerAPIConfig `yaml:"api"`
	RateLimit RateLimitConfig   `yaml:"rate_limit"`
}

type ExecutorConfig struct {
	Debug         bool     `yaml:"debug"`
	SendEmpty     bool     `yaml:"send_empty"`
	MaxPaperNum   int      `yaml:"max_paper_num"`
	Sources       []string `yaml:"source"`
	Reranker      string   `yaml:"reranker"`
	TokenizerType string   `yaml:"tokenizer_type"`
}

type Config struct {
	Zotero   ZoteroConfig   `yaml:"zotero"`
	Source   SourceConfig   `yaml:"source"`
	Email    EmailConfig    `yaml:"email"`
	LLM      LLMConfig      `yaml:"llm"`
	Reranker RerankerConfig `yaml:"reranker"`
	Executor ExecutorConfig `yaml:"executor"`
}

// innermostEnvRegex matches innermost ${...} without nested braces
var innermostEnvRegex = regexp.MustCompile(`\$\{([^{}]+)\}`)

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ExpandEnv replaces env placeholders in YAML content, resolving nested and default expressions.
func ExpandEnv(content string) string {
	for i := 0; i < 10; i++ {
		if !innermostEnvRegex.MatchString(content) {
			break
		}
		content = innermostEnvRegex.ReplaceAllStringFunc(content, func(match string) string {
			inner := match[2 : len(match)-1] // strip "${" and "}"
			inner = strings.TrimPrefix(inner, "oc.env:")
			inner = strings.TrimPrefix(inner, "oc.decode:")

			parts := strings.SplitN(inner, ",", 2)
			varName := strings.TrimSpace(parts[0])

			val, exists := os.LookupEnv(varName)
			if exists && val != "" {
				return val
			}

			if len(parts) == 2 {
				defVal := strings.TrimSpace(parts[1])
				if defVal == "null" {
					return ""
				}
				return defVal
			}

			if varName == "true" || varName == "false" || isDigits(varName) {
				return varName
			}

			return ""
		})
	}
	return content
}

// LoadConfigFromDir loads config files from configDir, merging base.yaml (or default.yaml / config.yaml) and custom.yaml (if present)
func LoadConfigFromDir(configDir string) (*Config, error) {
	var cfg Config

	basePath := filepath.Join(configDir, "base.yaml")
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		for _, alt := range []string{"default.yaml", "config.yaml"} {
			p := filepath.Join(configDir, alt)
			if _, err := os.Stat(p); err == nil {
				basePath = p
				break
			}
		}
	}

	baseBytes, err := readAndExpandFile(basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load base config %s: %w", basePath, err)
	}

	if err := yaml.Unmarshal(baseBytes, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse base config: %w", err)
	}

	customPath := filepath.Join(configDir, "custom.yaml")
	if _, err := os.Stat(customPath); err == nil {
		customBytes, err := readAndExpandFile(customPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load custom config %s: %w", customPath, err)
		}

		if err := yaml.Unmarshal(customBytes, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse custom config: %w", err)
		}
	}

	applyConfigDefaultsAndFallbacks(&cfg)

	// Validation checks
	if cfg.Zotero.UserID == "???" || cfg.Zotero.UserID == "" {
		return nil, fmt.Errorf("zotero.user_id is required and not configured")
	}
	if cfg.Zotero.APIKey == "???" || cfg.Zotero.APIKey == "" {
		return nil, fmt.Errorf("zotero.api_key is required and not configured")
	}

	return &cfg, nil
}

// LoadConfigFromFile loads a single config file and expands env variables.
func LoadConfigFromFile(path string) (*Config, error) {
	var cfg Config
	bytes, err := readAndExpandFile(path)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(bytes, &cfg); err != nil {
		return nil, err
	}

	applyConfigDefaultsAndFallbacks(&cfg)

	// Validation checks
	if cfg.Zotero.UserID == "???" || cfg.Zotero.UserID == "" {
		return nil, fmt.Errorf("zotero.user_id is required and not configured")
	}
	if cfg.Zotero.APIKey == "???" || cfg.Zotero.APIKey == "" {
		return nil, fmt.Errorf("zotero.api_key is required and not configured")
	}

	return &cfg, nil
}

func applyConfigDefaultsAndFallbacks(cfg *Config) {
	if cfg.Email.SMTPUser == "" {
		if cfg.Email.SMTPUsername != "" {
			cfg.Email.SMTPUser = cfg.Email.SMTPUsername
		} else {
			cfg.Email.SMTPUser = cfg.Email.Sender
		}
	}

	if cfg.LLM.API.Key == "" {
		if k := os.Getenv("LLM_API_KEY"); k != "" {
			cfg.LLM.API.Key = k
		} else if k := os.Getenv("GEMINI_API_KEY"); k != "" {
			cfg.LLM.API.Key = k
		} else if k := os.Getenv("OPENAI_API_KEY"); k != "" {
			cfg.LLM.API.Key = k
		}
	}

	if cfg.Reranker.API.Key == "" {
		if k := os.Getenv("EMBEDDING_API_KEY"); k != "" {
			cfg.Reranker.API.Key = k
		} else if cfg.LLM.API.Key != "" {
			cfg.Reranker.API.Key = cfg.LLM.API.Key
		} else if k := os.Getenv("GEMINI_API_KEY"); k != "" {
			cfg.Reranker.API.Key = k
		}
	}
}

func readAndExpandFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	bytes, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	expanded := ExpandEnv(string(bytes))
	return []byte(expanded), nil
}

// GetEnvAsBool retrieves boolean from environment or returns default.
func GetEnvAsBool(key string, defaultVal bool) bool {
	val, exists := os.LookupEnv(key)
	if !exists {
		return defaultVal
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return defaultVal
	}
	return b
}
