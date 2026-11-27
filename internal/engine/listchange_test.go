package engine

import (
	"fmt"
	"sync"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
)

// providers.yaml and mcp.yaml are lists, each read, changed and written whole: two
// changes at once, such as two models starred in a row, must both be kept.
func TestProviderAndMCPChangesAtOnceAreAllKept(t *testing.T) {
	e := openTest(t, t.TempDir())
	defer e.Close()
	p, err := e.SaveProvider(SaveProviderRequest{Name: "Local", Type: "ollama", BaseURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	const n = 20
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := e.SetModelFlags(p.ID, fmt.Sprintf("model-%d", i), true, false); err != nil {
				t.Error(err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := e.SaveMCPServer(SaveMCPServerRequest{Server: config.MCPServer{ID: fmt.Sprintf("server-%d", i), Transport: "http", URL: "https://example.com/mcp"}}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	e.mu.Lock()
	var models map[string]config.ModelSettings
	for _, c := range e.providers.configs {
		if c.ID == p.ID {
			models = c.Models
		}
	}
	e.mu.Unlock()
	servers := map[string]bool{}
	for _, s := range e.ListMCPServers() {
		servers[s.ID] = true
	}
	for i := range n {
		if !models[fmt.Sprintf("model-%d", i)].Favorite {
			t.Errorf("model-%d lost its star", i)
		}
		if !servers[fmt.Sprintf("server-%d", i)] {
			t.Errorf("server-%d was dropped", i)
		}
	}
}
