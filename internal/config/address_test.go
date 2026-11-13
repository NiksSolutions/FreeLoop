package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SEC-004: an address holding a credential is refused, whatever names its key.
func TestCheckAddress(t *testing.T) {
	for raw, bad := range map[string]bool{
		"":                           false,
		"https://api.example.com/v1": false,
		"https://search.example.com/?q={query}&key={key}": false,
		"https://api.example.com/v1?api_key=sk-123":       true,
		"https://api.example.com/v1?apikey=sk-123":        true,
		"https://api.example.com/v1?access-token=abc":     true,
		"https://mcp.exa.ai/mcp?exaApiKey=exa-123":        true,
		"https://mcp.example.com/mcp?authToken=abc":       true,
		"https://mcp.example.com/mcp?TAVILY_KEY=abc":      true,
		"https://me:pw@api.example.com/":                  true,
		"https://mcp.example.com/mcp?profile=work":        false,
		"https://mcp.example.com/mcp?monkey=1":            false,
	} {
		if err := CheckAddress("the address", raw); (err != nil) != bad {
			t.Errorf("CheckAddress(%q) = %v", raw, err)
		} else if err != nil && strings.Contains(err.Error(), "123") {
			t.Errorf("the error repeats the key: %v", err)
		}
	}
}

// SEC-004, MCP-001: an MCP server's address may not hold its key, neither saved
// from the app nor written into mcp.yaml.
func TestMCPServerAddressWithKeyIsRefused(t *testing.T) {
	const key = "exa-SUPERSECRETKEY0123456789"
	s := MCPServer{ID: "exa", Transport: "http", URL: "https://mcp.exa.ai/mcp?exaApiKey=" + key}
	err := s.Validate()
	if err == nil {
		t.Fatal("an address with a key validated")
	}
	if strings.Contains(err.Error(), key) {
		t.Errorf("the error repeats the key: %v", err)
	}
	if err := (MCPServer{ID: "u", Transport: "http", URL: "https://me:pw@mcp.example.com/mcp"}).Validate(); err == nil {
		t.Error("an address with a password validated")
	}
	dir := t.TempDir()
	if err := SaveMCP(dir, []MCPServer{s}); err == nil {
		t.Error("SaveMCP kept an address with a key")
	}
	file := "version: 1\nservers:\n  - id: exa\n    transport: http\n    url: https://mcp.exa.ai/mcp?exaApiKey=" + key + "\n  - id: plain\n    transport: http\n    url: https://mcp.example.com/mcp\n"
	if err := os.WriteFile(filepath.Join(dir, MCPFileName), []byte(file), 0o600); err != nil {
		t.Fatal(err)
	}
	servers, issues, err := LoadMCP(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 || servers[0].ID != "plain" {
		t.Errorf("servers = %+v", servers)
	}
	if len(issues) != 1 || strings.Contains(issues[0].Message, key) {
		t.Errorf("issues = %+v", issues)
	}
}

// SEC-004: mcp.registryURL holding a key is reported and not used.
func TestRegistryAddressWithKeyIsDropped(t *testing.T) {
	cfg, issues, err := Load(write(t, "version: 1\nmcp:\n  registryURL: https://registry.example.com/?token=abc123\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MCP.RegistryURL != "" {
		t.Errorf("registryURL = %q", cfg.MCP.RegistryURL)
	}
	if len(issues) != 1 || issues[0].Field != "mcp.registryURL" || strings.Contains(issues[0].Message, "abc123") {
		t.Errorf("issues = %+v", issues)
	}
}
