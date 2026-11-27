package engine

import (
	"context"
	"log"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/netguard"
)

// watch polls the hand-editable config files and reloads the ones that
// changed (SET-001). Polling needs no dependency and meets the two-second
// target; the files are few and small. Problems in an edited file are
// reported through ConfigIssues as at startup, never as a crash, and the
// app's own saves pass through here too, which is harmless.
func (e *Engine) watch(interval time.Duration) {
	defer e.watching.Done()
	t := time.NewTicker(interval)
	defer t.Stop()
	w := e.newConfigWatch()
	for {
		select {
		case <-e.quit:
			return
		case <-t.C:
		}
		// Each step on its own (NFR-REL-007): one that keeps failing leaves the others.
		e.safely("watching agent files", e.watchAgents)
		e.safely("the idle stop", func() { e.stopIdle(time.Now()) })
		e.safely("releasing transcripts", func() { e.releaseTranscripts(time.Now()) })
		e.safely("reloading config files", func() { e.reloadChanged(w) })
	}
}

// configWatch is what the watch has seen of the config files: the fingerprint each
// was last applied at, and each one's on the poll before.
type configWatch struct {
	applied, seen map[string]string
}

func (e *Engine) newConfigWatch() *configWatch {
	now := config.Fingerprint(e.dataDir)
	return &configWatch{applied: now, seen: maps.Clone(now)}
}

// reloadChanged rereads the config files that changed since they were applied and
// read the same on this poll as on the one before. An editor that truncates a file
// and then writes it can be caught between the two, and a file half written would
// apply for a poll: an empty rules.yaml has no deny rules, a cut config.yaml turns
// local-only mode off. With the poll every second, an edit applies within two. The
// fingerprint applied moves on first, so a file whose reload fails is not read again
// every round.
func (e *Engine) reloadChanged(w *configWatch) {
	now := config.Fingerprint(e.dataDir)
	before := w.seen
	w.seen = now
	var changed []string
	for _, f := range config.Changed(w.applied, now) {
		was, wasThere := before[f]
		is, isThere := now[f]
		if was != is || wasThere != isThere {
			continue
		}
		changed = append(changed, f)
		if isThere {
			w.applied[f] = is
		} else {
			delete(w.applied, f)
		}
	}
	if len(changed) == 0 {
		return
	}
	slices.Sort(changed)
	e.reload(changed)
	if e.onChange != nil {
		e.onChange(ConfigChange{Files: changed})
	}
}

// applyConfigFile reads config.yaml and puts it in force. Called with settingsMu held.
func (e *Engine) applyConfigFile() {
	print := config.FileFingerprint(e.dataDir, config.FileName)
	cfg, issues, err := config.Load(e.dataDir)
	if err != nil {
		issues = append(issues, config.Issue{File: config.FileName, Message: err.Error(), Unreadable: true})
	}
	e.mu.Lock()
	// A file that cannot be read keeps the settings in force, so a
	// half-saved edit does not turn off local-only mode or hooks.
	if !config.Unreadable(issues) {
		cfg.Revision = e.cfg.Revision + 1
		e.cfg = cfg
	}
	e.cfgPrint = print
	localOnly := e.cfg.Network.LocalOnly
	e.issues[issueConfig] = issues
	e.mu.Unlock()
	netguard.SetLocalOnly(localOnly)
}

// freshConfig puts config.yaml in force again when it changed on disk since the engine
// last read or wrote it, before the app changes it: the watch applies an edit made by
// hand only on its next polls, and the change would write over it (SET-001). Called
// with settingsMu held.
func (e *Engine) freshConfig() {
	print := config.FileFingerprint(e.dataDir, config.FileName)
	e.mu.Lock()
	same := print == e.cfgPrint
	e.mu.Unlock()
	if !same {
		e.applyConfigFile()
		e.dropBypass(context.Background())
	}
}

// reload rereads the config the changed files hold. Running sessions keep
// the provider and environment they started with; new sessions and the next
// permission check see the new settings.
func (e *Engine) reload(changed []string) {
	envs, profiles, styles := false, false, false
	for _, f := range changed {
		// The files come in the system's spelling; the folders are named with /, and
		// presets/harnesses is two of them, so on Windows they are compared with / too.
		under := func(dir string) bool { return strings.HasPrefix(filepath.ToSlash(f), dir+"/") }
		switch {
		case f == config.FileName:
			// Not between an app change's read of the config and its write.
			e.settingsMu.Lock()
			e.applyConfigFile()
			e.settingsMu.Unlock()
			e.dropBypass(context.Background())
		case f == config.ProvidersFileName:
			e.providersMu.Lock()
			if err := e.loadProviders(); err != nil {
				e.setIssues(issueProviders, []config.Issue{{File: config.ProvidersFileName, Message: err.Error()}})
			}
			e.learnKeyAddresses()
			e.checkKeyAddresses()
			e.providersMu.Unlock()
		case f == config.RulesFileName:
			e.rules.reload()
		case f == config.MCPFileName:
			e.mcpListMu.Lock()
			e.loadMCP()
			e.mcpListMu.Unlock()
		case f == config.ColorsFileName:
			e.loadColors()
		case under(config.StylesDir):
			styles = true
		case under(config.EnvironmentsDir):
			envs = true
		case under(config.ProfilesDir):
			profiles = true
		case under(config.OptimizersDir):
			e.loadOptimizers()
		case under(config.WorkflowsDir):
			e.loadWorkflows()
		case under(config.PresetsDir):
			e.loadPresets()
			profiles = true
		default:
			log.Printf("engine: no reload for %s", f)
		}
	}
	if styles {
		e.loadStyles()
	}
	if envs {
		e.loadEnvironments()
	}
	if profiles {
		e.loadProfiles()
	}
}
