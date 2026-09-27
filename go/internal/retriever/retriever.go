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

package retriever

import (
	"context"
	"fmt"
	"sync"

	"github.com/exTerEX/zotero-arxiv-daily/go/internal/config"
	"github.com/exTerEX/zotero-arxiv-daily/go/internal/model"
)

// Retriever defines the interface for fetching papers from any repository.
type Retriever interface {
	RetrievePapers(ctx context.Context) ([]model.Paper, error)
}

type Factory func(cfg *config.Config) (Retriever, error)

var (
	registryMu sync.RWMutex
	registry   = make(map[string]Factory)
)

// Register registers a new retriever factory.
func Register(name string, factory Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[name] = factory
}

// Get retrieves a retriever by name.
func Get(name string, cfg *config.Config) (Retriever, error) {
	registryMu.RLock()
	factory, exists := registry[name]
	registryMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("retriever %s not registered", name)
	}

	return factory(cfg)
}
