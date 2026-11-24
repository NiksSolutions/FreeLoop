package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/release"
)

// signedRelease is a release folder of version 1.2.3 with two files, its list and the
// list's signature, and the public key that checks it.
func signedRelease(t *testing.T) (dir, pub string) {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	dir = t.TempDir()
	var sums strings.Builder
	for _, f := range []struct{ name, body string }{{"Freeloop.dmg", "the mac's"}, {"Freeloop-1.2.3-windows-amd64-portable.zip", "windows'"}} {
		if err := os.WriteFile(filepath.Join(dir, f.name), []byte(f.body), 0o644); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte(f.body))
		fmt.Fprintf(&sums, "%s  %s\n", hex.EncodeToString(sum[:]), f.name)
	}
	os.WriteFile(filepath.Join(dir, release.SumsName), []byte(sums.String()), 0o644)
	os.WriteFile(filepath.Join(dir, release.SigName), []byte(release.Sign(key, "1.2.3", []byte(sums.String()))), 0o644)
	return dir, base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey))
}

func verifyOut(dir, pub string, more ...string) (string, error) {
	var out bytes.Buffer
	err := run(append([]string{"verify", "-version", "1.2.3", "-dir", dir, "-key", pub}, more...), &out)
	return out.String(), err
}

// SEC-021: verify checks the signature over the whole list and every listed file; a
// tester with only some of the files says so with -ignore-missing, and the files that
// are there are still checked, and named.
func TestVerifyChecksTheSignatureAndTheFilesThatAreThere(t *testing.T) {
	dir, pub := signedRelease(t)
	out, err := verifyOut(dir, pub)
	if err != nil || !strings.Contains(out, "2 files as listed") {
		t.Fatalf("a whole release: %q, %v", out, err)
	}

	// Only the Mac's file, as a tester downloads it.
	os.Remove(filepath.Join(dir, "Freeloop-1.2.3-windows-amd64-portable.zip"))
	if _, err := verifyOut(dir, pub); err == nil || !strings.Contains(err.Error(), "-ignore-missing") {
		t.Errorf("an absent file without the flag: %v", err)
	}
	out, err = verifyOut(dir, pub, "-ignore-missing")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "checked Freeloop.dmg") || !strings.Contains(out, "not here: Freeloop-1.2.3-windows-amd64-portable.zip") || !strings.Contains(out, "signed by the project") {
		t.Errorf("what it says it checked:\n%s", out)
	}

	// A file that is there and differs still fails.
	os.WriteFile(filepath.Join(dir, "Freeloop.dmg"), []byte("someone else's"), 0o644)
	if _, err := verifyOut(dir, pub, "-ignore-missing"); err == nil || !strings.Contains(err.Error(), "Freeloop.dmg") {
		t.Errorf("a changed file: %v", err)
	}
	os.WriteFile(filepath.Join(dir, "Freeloop.dmg"), []byte("the mac's"), 0o644)

	// The signature is over the whole list: one cut to the files at hand fails.
	sums, _ := os.ReadFile(filepath.Join(dir, release.SumsName))
	cut := strings.SplitAfter(string(sums), "\n")[0]
	os.WriteFile(filepath.Join(dir, release.SumsName), []byte(cut), 0o644)
	if _, err := verifyOut(dir, pub, "-ignore-missing"); err == nil || !strings.Contains(err.Error(), "not signed") {
		t.Errorf("a list cut to the files present: %v", err)
	}
	os.WriteFile(filepath.Join(dir, release.SumsName), sums, 0o644)

	// Another version's, or another key's, fails.
	var out2 bytes.Buffer
	if err := run([]string{"verify", "-version", "1.2.4", "-dir", dir, "-key", pub, "-ignore-missing"}, &out2); err == nil {
		t.Error("another version's signature passed")
	}
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := verifyOut(dir, base64.StdEncoding.EncodeToString(other.Public().(ed25519.PublicKey)), "-ignore-missing"); err == nil {
		t.Error("another key passed")
	}

	// With none of the listed files there, nothing but the list was checked: that is
	// not a download checked, as a file saved under another name would be.
	os.Remove(filepath.Join(dir, "Freeloop.dmg"))
	if out, err := verifyOut(dir, pub, "-ignore-missing"); err == nil || !strings.Contains(err.Error(), "none of the files") {
		t.Errorf("no listed file present: %q, %v", out, err)
	}
}

// The flag is named in the tool's usage.
func TestUsageNamesTheFlag(t *testing.T) {
	err := run(nil, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "-ignore-missing") {
		t.Errorf("usage: %v", err)
	}
}
