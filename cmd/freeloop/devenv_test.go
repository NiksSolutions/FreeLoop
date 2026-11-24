package main

import (
	"github.com/FinanceNik/Freeloop/internal/secrets"
	"slices"
	"testing"
)

func TestDevValueIgnoresTheEnvironmentWhenShipped(t *testing.T) {
	env := map[string]string{
		"FREELOOP_DEV": "1", "FREELOOP_DEV_GH": "/tmp/gh", "FREELOOP_DEV_SECRETS": "memory",
		"FREELOOP_DEV_BACKUP": "/tmp/b.json", "FREELOOP_RELEASE_API": "http://127.0.0.1:1", "FREELOOP_HOME": "/tmp/home",
	}
	getenv := func(name string) string { return env[name] }
	for name, value := range env {
		if got := devValue(false, name, getenv); got != value {
			t.Errorf("development build: %s = %q, want %q", name, got, value)
		}
		if got := devValue(true, name, getenv); got != "" {
			t.Errorf("shipped build: %s = %q, want it ignored", name, got)
		}
		if !isDevSwitch(name) {
			t.Errorf("%s is not counted as a development switch", name)
		}
	}
}

func TestIgnoredDevSwitchesNamesOnlySwitches(t *testing.T) {
	environ := []string{"HOME=/Users/x", "FREELOOP_HOME=/tmp/secret-path", "FREELOOP_DEV=1", "FREELOOP_DEVELOPER=1",
		"FREELOOP_DEV_GH=/bin/gh", "FREELOOP_RELEASE_API=http://x", "FREELOOP_PORT=3000", "FREELOOP_DEV=1"}
	want := []string{"FREELOOP_DEV", "FREELOOP_DEV_GH", "FREELOOP_HOME", "FREELOOP_RELEASE_API"}
	if got := ignoredDevSwitches(true, environ); !slices.Equal(got, want) {
		t.Errorf("shipped: %v, want %v", got, want)
	}
	if got := ignoredDevSwitches(false, environ); got != nil {
		t.Errorf("development build ignores %v", got)
	}
}

// devEnv follows the link-time flag: a plain `go test` reads the switches, and
// one linked as the shipped app is ignores them. A test binary names this package
// by its import path, not main:
// go test -ldflags=-X=github.com/FinanceNik/Freeloop/cmd/freeloop.shipped=1 ./cmd/freeloop
func TestDevEnvFollowsTheLinkTimeFlag(t *testing.T) {
	t.Setenv("FREELOOP_DEV_GH", "/tmp/gh")
	want := "/tmp/gh"
	if shipped == "1" {
		want = ""
	}
	t.Logf("shipped = %q", shipped)
	if got := devEnv("FREELOOP_DEV_GH"); got != want {
		t.Errorf("shipped = %q: devEnv = %q, want %q", shipped, got, want)
	}
}

// A development build on scratch data keeps its secrets in memory when told to, the
// packaged app as well as the served one: it ran memcheck and a hand check on
// scratch data against the login keychain, and macOS asked the user. A shipped build
// keeps them in the keychain whatever the environment says.
func TestDevSecretsStayInMemory(t *testing.T) {
	t.Setenv("FREELOOP_DEV_SECRETS", "memory")
	_, inMemory := secretStore().(*secrets.Memory)
	if want := shipped != "1"; inMemory != want {
		t.Errorf("shipped = %q: secrets in memory = %v, want %v", shipped, inMemory, want)
	}
	t.Setenv("FREELOOP_DEV_SECRETS", "")
	if _, ok := secretStore().(*secrets.Memory); ok {
		t.Error("without the switch, secrets are kept in memory")
	}
}
