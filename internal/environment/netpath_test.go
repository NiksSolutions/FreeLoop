package environment

import "testing"

// Security review, 2026-11-13: on Windows, resolving `\\host\share\x` to judge it
// opened an SMB connection to that host, which sends the user's NTLM hash, before the
// broker saw the call. These spellings name a place on another computer or a device,
// and Resolve leaves them alone; the table runs on every system.
func TestFarPath(t *testing.T) {
	for _, c := range []struct {
		in   string
		want bool
	}{
		{`\\attacker.example\s\x`, true},
		{`//attacker.example/s/x`, true},
		{`\/attacker.example\s`, true},
		{`\\?\UNC\attacker.example\s\x`, true},
		{`//?/UNC/attacker.example/s/x`, true},
		{`\\.\UNC\attacker.example\s\x`, true},
		{`\\.\pipe\x`, true},
		{`\\?\GLOBALROOT\Device\Mup\attacker.example\s`, true},
		{`\\?\Volume{0b1c}\x`, true},
		{`\\attacker.example`, true},
		{`\\?\C:\Users\me\p`, false},
		{`\\.\D:\x`, false},
		{`C:\Users\me\p`, false},
		{`C:x`, false},
		{`\Users\me`, false},
		{`/Users/me/p`, false},
		{`relative\x`, false},
		{``, false},
	} {
		if got := farPath(c.in); got != c.want {
			t.Errorf("farPath(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
