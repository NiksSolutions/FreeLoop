package engine

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	goruntime "runtime"
	"runtime/pprof"
	"strconv"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/harness/builtin"
)

// ReportLogLines is how many of the log's last lines a problem report carries.
const ReportLogLines = 200

// reportGoroutineLines bounds what a report says the app's goroutines are doing.
const reportGoroutineLines = 400

// appModule begins every name and file of the app's own code in a stack.
const appModule = "github.com/FinanceNik/Freeloop/"

// ProblemReport is the text "Copy a report" puts on the clipboard, for an issue: the
// app's version, this computer, the session's agent, provider and model when a session is
// given, how far behind the transcripts the counts kept from them are, what the app's
// goroutines are doing, and the end of the log. It goes nowhere by itself (NFR-PRIV-001), and
// the user reads it before pasting it. The log is redacted again here, since a
// crash trace reaches the log file without passing the redaction the app's own lines
// do, and the home folder's path, which holds the user's name, reads ~.
func (e *Engine) ProblemReport(ctx context.Context, sessionID string) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Freeloop %s (%s/%s, %s)\n", Build().Label, goruntime.GOOS, goruntime.GOARCH, goruntime.Version())
	system := "unknown"
	if e.systemInfo != nil {
		system = e.systemInfo()
	}
	fmt.Fprintf(&b, "System: %s\n", system)
	if sessionID != "" {
		if ses, err := e.store.GetSession(ctx, sessionID); err == nil {
			agent := ses.HarnessID
			if agent == builtin.ID {
				agent = "Built-in"
			} else if p, err := e.preset(ses.HarnessID); err == nil && p.Name != "" {
				agent = p.Name
			}
			fmt.Fprintf(&b, "Session: %s", agent)
			if ses.ProviderID != "" {
				fmt.Fprintf(&b, " on %s", e.providerName(ses.ProviderID))
			}
			if ses.Model != "" {
				fmt.Fprintf(&b, ", model %s", ses.Model)
			}
			fmt.Fprintf(&b, ", %s mode, %s\n", ses.Mode, ses.Status)
		}
	}
	// A count kept from the transcripts that stopped moving shows here.
	if behind, err := e.store.Behind(ctx); err == nil {
		b.WriteString("\nCounts kept from the transcripts:\n")
		for _, l := range behind {
			fmt.Fprintf(&b, "- %s: ", l.Name)
			switch {
			case l.Sessions == 0:
				b.WriteString("up to date")
			case l.Sessions == 1:
				fmt.Fprintf(&b, "1 session, %d events to read", l.Events)
			default:
				fmt.Fprintf(&b, "%d sessions, %d events to read", l.Sessions, l.Events)
			}
			if l.Sessions > 0 && l.OnRequest {
				b.WriteString(" (brought up to date when the Usage page asks)")
			}
			b.WriteByte('\n')
		}
	}
	// An app that seems stuck, a count that no longer moves, say, shows where here.
	if doing, total := goroutineSnapshot(); doing != "" {
		fmt.Fprintf(&b, "\nWhat the app is doing (%d goroutines; those in Freeloop's code, by where they stand):\n%s\n", total, doing)
	}
	lines, err := config.LogTail(e.dataDir, ReportLogLines)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(&b, "\nThe log's last %d lines:\n", len(lines))
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return e.anonymize(e.redactText(b.String())), nil
}

// goroutineSnapshot says what the app's own goroutines are doing: each group standing at
// the same calls, with how many, innermost call first as Go prints them, and how many
// goroutines there are in all. Calls are named with their file and line, never with
// their values; groups wholly outside the app's code are left out.
func goroutineSnapshot() (string, int) {
	var raw bytes.Buffer
	if err := pprof.Lookup("goroutine").WriteTo(&raw, 1); err != nil {
		return "", 0
	}
	total := 0
	var lines []string
	for _, block := range strings.Split(raw.String(), "\n\n") {
		block = strings.TrimSpace(block)
		if head, rest, ok := strings.Cut(block, "\n"); ok && strings.HasPrefix(head, "goroutine profile: total ") {
			total, _ = strconv.Atoi(strings.TrimPrefix(head, "goroutine profile: total "))
			block = rest
		}
		if !strings.Contains(block, appModule) {
			continue
		}
		// "2 @ 0x… 0x…", then a line per call: "#\t0x…\tname+0x…\tfile:line".
		head, calls, _ := strings.Cut(block, "\n")
		count, _, _ := strings.Cut(head, " @ ")
		lines = append(lines, count+" ×")
		for _, call := range strings.Split(calls, "\n") {
			// By tabs, since a folder's name can hold spaces.
			var fields []string
			for _, f := range strings.Split(strings.TrimPrefix(call, "#"), "\t") {
				if f = strings.TrimSpace(f); f != "" {
					fields = append(fields, f)
				}
			}
			if len(fields) < 3 || !strings.HasPrefix(fields[0], "0x") {
				continue
			}
			name, _, _ := strings.Cut(fields[1], "+0x")
			lines = append(lines, strings.ReplaceAll("    "+name+" ("+fields[len(fields)-1]+")", appModule, ""))
		}
	}
	if len(lines) > reportGoroutineLines {
		lines = append(lines[:reportGoroutineLines], fmt.Sprintf("    … and %d more lines", len(lines)-reportGoroutineLines))
	}
	return strings.Join(lines, "\n"), total
}

// providerName is a provider's name as the user gave it, else its ID.
func (e *Engine) providerName(id string) string {
	for _, p := range e.ListProviders() {
		if p.ID == id && p.Name != "" {
			return p.Name
		}
	}
	return id
}

// anonymize writes the home folder as ~, so the user's account name stays out.
func (e *Engine) anonymize(s string) string {
	return anonymizeHome(goruntime.GOOS, e.home, s)
}

// anonymizeHome writes home in s as ~. On Windows the folder appears in several
// spellings, C:\Users\a, C:/Users/a from git, /c/Users/a from Git Bash and
// C:\\Users\\a in JSON, in any case, since the file system ignores it
// (NFR-XPLAT-003).
func anonymizeHome(goos, home, s string) string {
	if goos != "windows" {
		if home == "" || home == "/" {
			return s
		}
		return strings.ReplaceAll(s, strings.TrimRight(home, "/"), "~")
	}
	home = strings.TrimRight(home, `\/`)
	if len(home) < 3 {
		return s
	}
	slashed := strings.ReplaceAll(home, `\`, "/")
	spellings := []string{strings.ReplaceAll(home, `\`, `\\`), home, slashed, bashPath("windows", home)}
	var alts []string
	for _, sp := range spellings {
		alts = append(alts, regexp.QuoteMeta(sp))
	}
	return regexp.MustCompile("(?i)"+strings.Join(alts, "|")).ReplaceAllString(s, "~")
}
