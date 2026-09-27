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

package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/exTerEX/zotero-arxiv-daily/go/internal/config"
	"github.com/exTerEX/zotero-arxiv-daily/go/internal/executor"

	// Register retrievers
	_ "github.com/exTerEX/zotero-arxiv-daily/go/internal/retriever"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	versionFlag := flag.Bool("version", false, "Print application version information and exit")
	configPath := flag.String("config", "config", "Path to configuration directory (containing base.yaml and custom.yaml) or a single config file")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("zotero-arxiv-daily version %s (commit %s, built at %s)\n", version, commit, date)
		os.Exit(0)
	}

	// Load configuration
	var cfg *config.Config
	var err error

	info, err := os.Stat(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error checking config path %s: %v\n", *configPath, err)
		os.Exit(1)
	}

	if info.IsDir() {
		cfg, err = config.LoadConfigFromDir(*configPath)
	} else {
		cfg, err = config.LoadConfigFromFile(*configPath)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Configure default slog handler
	var programLevel = new(slog.LevelVar)
	if cfg.Executor.Debug {
		programLevel.Set(slog.LevelDebug)
	} else {
		programLevel.Set(slog.LevelInfo)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: programLevel,
	}))
	slog.SetDefault(logger)

	slog.Info("Configuration loaded successfully")
	if cfg.Executor.Debug {
		slog.Debug("Debug logging is enabled")
	}

	// Create and run the Executor
	exec, err := executor.New(cfg)
	if err != nil {
		slog.Error("Failed to initialize executor", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if err := exec.Run(ctx); err != nil {
		slog.Error("Workflow execution failed", "error", err)
		os.Exit(1)
	}

	slog.Info("Workflow completed successfully")
}
