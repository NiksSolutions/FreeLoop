package engine

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// SET-005: restoring a backup on a fresh install recreates the providers,
// asking for the keys the Keychain does not have, and the profiles; the
// backup holds no secret.
func TestConfigBackupRestore(t *testing.T) {
	ctx := context.Background()
	a := openTest(t, t.TempDir())
	defer a.Close()
	if _, err := a.SaveProvider(SaveProviderRequest{Name: "Work", Type: "anthropic", APIKey: "sk-ant-secret-1234567890"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveProfile(config.Profile{ID: "review", Name: "Review", Mode: "plan"}); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "freeloop-config.json")
	if err := a.ExportConfig(ctx, backup); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(backup)
	if strings.Contains(string(b), "sk-ant-secret") || !strings.Contains(string(b), "keychain://") {
		t.Fatalf("the backup:\n%s", b)
	}

	fresh := openTest(t, t.TempDir())
	defer fresh.Close()
	res, err := fresh.ImportConfig(ctx, backup)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(fresh.ListProviders(), func(p ProviderInfo) bool { return p.Name == "Work" }) {
		t.Errorf("providers = %+v", fresh.ListProviders())
	}
	if !slices.ContainsFunc(fresh.ListProfiles(), func(p config.Profile) bool { return p.ID == "review" && p.Mode == "plan" }) {
		t.Errorf("profiles = %+v", fresh.ListProfiles())
	}
	if !slices.Equal(res.NeedKeys, []string{"Work"}) {
		t.Errorf("need keys = %v", res.NeedKeys)
	}
}

// A restore waits for a change to the settings already under way, rather than writing
// config.yaml between that change's read and its write, which put the old file back
// (issue 091's locks).
func TestARestoreWaitsForAChangeUnderWay(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e, err := Open(Options{DataDir: dir, Secrets: secrets.NewMemory(), Home: t.TempDir(), WatchInterval: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if _, err := e.ChangeSettings(`{"ui":{"language":"nl"}}`); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "backup.zip")
	if err := e.ExportConfig(ctx, backup); err != nil {
		t.Fatal(err)
	}
	if _, err := e.ChangeSettings(`{"ui":{"language":"de"}}`); err != nil {
		t.Fatal(err)
	}
	read, release := make(chan struct{}), make(chan struct{})
	changed := make(chan error, 1)
	go func() {
		_, err := e.updateSettings(func(c *config.Config) error {
			close(read)
			<-release
			c.UI.FontSize = 18
			return nil
		})
		changed <- err
	}()
	<-read
	restored := make(chan error, 1)
	go func() {
		_, err := e.ImportConfig(ctx, backup)
		restored <- err
	}()
	time.Sleep(200 * time.Millisecond)
	close(release)
	if err := <-changed; err != nil {
		t.Fatal(err)
	}
	if err := <-restored; err != nil {
		t.Fatal(err)
	}
	if c, _, _ := config.Load(dir); c.UI.Language != "nl" {
		t.Errorf("after the restore the file says %q", c.UI.Language)
	}
	if e.Config().UI.Language != "nl" {
		t.Errorf("after the restore the engine has %q", e.Config().UI.Language)
	}
}
