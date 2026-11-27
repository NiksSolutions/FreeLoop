package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/secrets"
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

// providers.yaml and mcp.yaml edited by hand a moment ago, before the watch applied
// them, are not written over by the next change made in the app (SET-001).
func TestHandEditsOfTheListsAreNotWrittenOver(t *testing.T) {
	dir := t.TempDir()
	e, err := Open(Options{DataDir: dir, Secrets: secrets.NewMemory(), Home: t.TempDir(), WatchInterval: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	p, err := e.SaveProvider(SaveProviderRequest{Name: "Local", Type: "ollama", BaseURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.SaveMCPServer(SaveMCPServerRequest{Server: config.MCPServer{ID: "first", Transport: "http", URL: "https://example.com/mcp"}}); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Second)
	cfgs, _, _ := config.LoadProviders(dir)
	cfgs[0].Models = map[string]config.ModelSettings{"by-hand": {Favorite: true}}
	if err := config.SaveProviders(dir, cfgs); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(filepath.Join(dir, config.ProvidersFileName), later, later)
	servers, _, _ := config.LoadMCP(dir)
	servers = append(servers, config.MCPServer{ID: "by-hand", Transport: "http", URL: "https://example.org/mcp"})
	if err := config.SaveMCP(dir, servers); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(filepath.Join(dir, config.MCPFileName), later, later)

	if err := e.SetModelFlags(p.ID, "in-app", true, false); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SaveMCPServer(SaveMCPServerRequest{Server: config.MCPServer{ID: "in-app", Transport: "http", URL: "https://example.net/mcp"}}); err != nil {
		t.Fatal(err)
	}
	cfgs, _, _ = config.LoadProviders(dir)
	if !cfgs[0].Models["by-hand"].Favorite || !cfgs[0].Models["in-app"].Favorite {
		t.Errorf("models in providers.yaml: %+v", cfgs[0].Models)
	}
	ids := map[string]bool{}
	servers, _, _ = config.LoadMCP(dir)
	for _, s := range servers {
		ids[s.ID] = true
	}
	if !ids["first"] || !ids["by-hand"] || !ids["in-app"] {
		t.Errorf("servers in mcp.yaml: %v", ids)
	}
}
