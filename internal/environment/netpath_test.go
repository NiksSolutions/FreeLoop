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

// On Windows these names are devices in any folder, so a read of C:\p\CON waits for
// typing and a write of C:\p\COM1 reaches a port; the table runs on every system.
func TestDosDevice(t *testing.T) {
	for in, want := range map[string]bool{
		`C:\p\NUL`: true, `C:\p\nul.txt`: true, `con`: true, `C:/p/Con.tar.gz`: true, `aux`: true, `PRN`: true,
		`COM1`: true, `lpt9.log`: true, `COM¹`: true, `NUL `: true, `NUL .txt`: true, `nul:stream`: true,
		`CONIN$`: true, `conout$`: true, `CLOCK$`: true,
		`C:\NUL\x.txt`: false, `null`: false, `console.go`: false, `COM10`: false, `LPT`: false, `aux_x`: false, `x.nul`: false, ``: false,
	} {
		if got := dosDevice(in); got != want {
			t.Errorf("dosDevice(%q) = %v, want %v", in, got, want)
		}
	}
	for in, want := range map[string]bool{
		".git.": true, ".git::$INDEX_ALLOCATION": true, "GIT~1": true, "git~12": true, ".git ": true, "nul": true, "a:b": true,
		".git": false, "a~b": false, "~": false, "x.txt": false, "ünï": false,
	} {
		if got := dosAlias(in); got != want {
			t.Errorf("dosAlias(%q) = %v, want %v", in, got, want)
		}
	}
}
