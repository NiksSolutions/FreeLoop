package engine

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
)

// What a workflow run cost (WFL-015): from the usage each of its sessions recorded,
// priced as the Usage page prices it, each call at the price of its day. Planning is the session
// the run was started from, up to the moment it began; Issues and Reviews are the
// run's sessions. An unpriced model, or an agent that gave no tokens, is said, never
// counted as nothing.

// CostByModel is what one model cost in a part of a run.
type CostByModel struct {
	ProviderID string  `json:"providerId"`
	Model      string  `json:"model"`
	Cost       float64 `json:"cost"`
	Priced     bool    `json:"priced"`
}

// CostPart is a part of a run's cost: the priced sum, its models, and whether
// anything in it has no price or no tokens.
type CostPart struct {
	Cost     float64       `json:"cost"`
	Unpriced bool          `json:"unpriced"`
	Models   []CostByModel `json:"models"`
}

// IssueCost is an issue's session and review, with the commit the issue ended at.
type IssueCost struct {
	CostPart
	Issue  string `json:"issue"`
	Commit string `json:"commit"`
}

// RunCost is a run's cost, in parts and by issue.
type RunCost struct {
	RunID    string      `json:"runId"`
	Planning CostPart    `json:"planning"`
	Issues   CostPart    `json:"issues"`
	Reviews  CostPart    `json:"reviews"`
	Total    float64     `json:"total"`
	Unpriced bool        `json:"unpriced"`
	PerIssue []IssueCost `json:"perIssue"`
}

// RunCost says what a run cost.
func (e *Engine) RunCost(ctx context.Context, runID string) (RunCost, error) {
	all, err := e.runs()
	if err != nil {
		return RunCost{}, err
	}
	i := slices.IndexFunc(all.Runs, func(r config.AFKRun) bool { return r.ID == runID })
	if i < 0 {
		return RunCost{}, errors.New("no such run")
	}
	return e.runCost(ctx, all.Runs[i]), nil
}

func (e *Engine) runCost(ctx context.Context, r config.AFKRun) RunCost {
	prices := config.EffectivePrices(e.Config())
	out := RunCost{RunID: r.ID, PerIssue: []IssueCost{}}
	if r.PlanningSession != "" {
		out.Planning = e.sessionCost(ctx, prices, r.StartedAt, r.PlanningSession)
	}
	var issues, reviews []string
	for _, s := range r.Steps {
		issues, reviews = append(issues, s.SessionID), append(reviews, s.ReviewID)
		out.PerIssue = append(out.PerIssue, IssueCost{CostPart: e.sessionCost(ctx, prices, time.Time{}, s.SessionID, s.ReviewID), Issue: s.Issue, Commit: s.Commit})
	}
	out.Issues = e.sessionCost(ctx, prices, time.Time{}, issues...)
	out.Reviews = e.sessionCost(ctx, prices, time.Time{}, reviews...)
	out.Total = out.Planning.Cost + out.Issues.Cost + out.Reviews.Cost
	out.Unpriced = out.Planning.Unpriced || out.Issues.Unpriced || out.Reviews.Unpriced
	return out
}

// issueCost is the cost of a run's step for an issue.
func (e *Engine) issueCost(ctx context.Context, s config.AFKStep) IssueCost {
	return IssueCost{CostPart: e.sessionCost(ctx, config.EffectivePrices(e.Config()), time.Time{}, s.SessionID, s.ReviewID), Issue: s.Issue, Commit: s.Commit}
}

// sessionCost prices what the sessions' model calls used, before a moment when one
// is given, by model.
func (e *Engine) sessionCost(ctx context.Context, prices []config.Price, before time.Time, sids ...string) CostPart {
	var calls []usedOn
	unreported := false
	for _, sid := range sids {
		if sid == "" {
			continue
		}
		ses, err := e.store.GetSession(ctx, sid)
		if err != nil {
			continue
		}
		evs, err := e.store.Events(ctx, sid, 0)
		if err != nil {
			continue
		}
		for _, ev := range evs {
			if ev.Type != event.UsageUpdate || (!before.IsZero() && !ev.TS.Before(before)) {
				continue
			}
			var d event.UsageData
			if json.Unmarshal(ev.Data, &d) != nil {
				continue
			}
			if d.Measured == event.MeasuredNone {
				unreported = true
				continue
			}
			// The day as the store counts usage by day.
			calls = append(calls, usedOn{provider: cmp.Or(d.ProviderID, ses.ProviderID), model: cmp.Or(d.Model, ses.Model),
				day: ev.TS.In(time.Local).Format(time.DateOnly), d: d})
		}
	}
	part := priceCalls(prices, calls)
	part.Unpriced = part.Unpriced || unreported
	return part
}

// usedOn is what a model call used, and on which day (YYYY-MM-DD).
type usedOn struct {
	provider, model, day string
	d                    event.UsageData
}

// priceCalls sums calls by model, each at the price that held on its day, as the
// Usage page prices them (config.FindPriceOn).
func priceCalls(prices []config.Price, calls []usedOn) CostPart {
	type key struct{ provider, model, day string }
	type tokens struct{ in, out, cr, cw, cw1h int64 }
	used := map[key]*tokens{}
	var order []key
	for _, c := range calls {
		k := key{c.provider, c.model, c.day}
		if c.d.OnePrompt() {
			k.model = config.PricedModel(k.model, c.d.InputTokens+c.d.CacheReadTokens+c.d.CacheWriteTokens)
		}
		t := used[k]
		if t == nil {
			t = &tokens{}
			used[k] = t
			order = append(order, k)
		}
		t.in, t.out, t.cr, t.cw, t.cw1h = t.in+c.d.InputTokens, t.out+c.d.OutputTokens, t.cr+c.d.CacheReadTokens, t.cw+c.d.CacheWriteTokens, t.cw1h+c.d.CacheWrite1hTokens
	}
	part := CostPart{Models: []CostByModel{}}
	index := map[[2]string]int{}
	for _, k := range order {
		i, seen := index[[2]string{k.provider, k.model}]
		if !seen {
			i = len(part.Models)
			index[[2]string{k.provider, k.model}] = i
			part.Models = append(part.Models, CostByModel{ProviderID: k.provider, Model: k.model, Priced: true})
		}
		m := &part.Models[i]
		if p, ok := config.FindPriceOn(prices, k.provider, k.model, k.day); ok {
			t := used[k]
			c := p.Cost(t.in, t.out, t.cr, t.cw, t.cw1h)
			m.Cost += c
			part.Cost += c
		} else {
			m.Priced = false
			part.Unpriced = true
		}
	}
	return part
}

// runWork is the run, issue and commit a session worked on.
type runWork struct{ run, label, issue, commit string }

// runSessions maps the sessions of AFK runs, their issues' and reviews', to what they
// worked on.
func (e *Engine) runSessions(ctx context.Context) map[string]runWork {
	out := map[string]runWork{}
	all, err := e.runs()
	if err != nil {
		return out
	}
	names := map[string]string{}
	if projects, err := e.store.ListProjects(ctx); err == nil {
		for _, p := range projects {
			names[p.ID] = p.Name
		}
	}
	for _, r := range all.Runs {
		label := fmt.Sprintf("%s · %s", cmp.Or(names[r.ProjectID], filepath.Base(r.ProjectRoot)), r.StartedAt.Local().Format("2006-01-02 15:04"))
		for _, s := range r.Steps {
			for _, sid := range []string{s.SessionID, s.ReviewID} {
				if sid != "" {
					out[sid] = runWork{run: r.ID, label: label, issue: s.Issue, commit: s.Commit}
				}
			}
		}
	}
	return out
}

// pullRequestCost is the line a pull request drafted from a run's branch carries:
// what the run cost, by part and model. Empty for a session of no run.
func (e *Engine) pullRequestCost(ctx context.Context, sid string) string {
	w, ok := e.runSessions(ctx)[sid]
	if !ok {
		return ""
	}
	rc, err := e.RunCost(ctx, w.run)
	if err != nil {
		return ""
	}
	var parts []string
	for _, p := range []struct {
		name string
		part CostPart
	}{{"Planning", rc.Planning}, {"Issues", rc.Issues}, {"Reviews", rc.Reviews}} {
		if len(p.part.Models) == 0 {
			continue
		}
		var models []string
		for _, m := range p.part.Models {
			// An agent that names no model adds no empty name.
			if m.Model != "" && !slices.Contains(models, m.Model) {
				models = append(models, m.Model)
			}
		}
		line := fmt.Sprintf("%s %s", p.name, costWords(p.part.Cost, p.part.Unpriced))
		if len(models) > 0 {
			line += " on " + strings.Join(models, ", ")
		}
		parts = append(parts, line)
	}
	if len(parts) == 0 {
		return ""
	}
	parts = append(parts, "total "+costWords(rc.Total, rc.Unpriced))
	return "Cost of this AFK run, estimated by Freeloop: " + strings.Join(parts, " · ") + "."
}

// costWords writes an amount in US dollars, saying when part of it has no price.
func costWords(cost float64, unpriced bool) string {
	if unpriced && cost == 0 {
		return "unpriced"
	}
	out := fmt.Sprintf("$%.2f", cost)
	if cost > 0 && cost < 0.01 {
		out = "<$0.01"
	}
	if unpriced {
		out += " + unpriced"
	}
	return out
}
