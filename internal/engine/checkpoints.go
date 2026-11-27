package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// Checkpoints (SES-014): before each prompt the session's folder is
// snapshotted into a git store of the session's own, in its data folder
// (not the project's repository, which is never touched, and which need not
// exist). The store honours the project's .gitignore, so dependencies and
// build output stay out. "Rewind to here" restores the files of a
// checkpoint, after snapshotting the current state, and cuts the built-in
// harness's context back to that point.

// checkpointTimeout bounds one snapshot; a folder too big to snapshot in
// time turns checkpoints off for the session.
const checkpointTimeout = 90 * time.Second

// History is implemented by harness sessions whose context can be cut back
// to a length (the built-in harness).
type History interface {
	HistoryLen() int
	TruncateHistory(n int)
}

func (e *Engine) checkpointStore(sid string) string {
	return filepath.Join(e.dataDir, "sessions", sid, "checkpoints.git")
}

// shadowBase is the start of a git command on the session's checkpoint store. A
// snapshot holds the files as they are: core.autocrlf, which Git for Windows turns
// on, would store \r\n as \n and give a rewind back \r\n where there was none.
func (e *Engine) shadowBase(sid string, host *environment.Local) []string {
	return []string{"--git-dir=" + e.checkpointStore(sid), "--work-tree=" + host.Root(), "-c", "user.name=Freeloop", "-c", "user.email=freeloop@localhost", "-c", "commit.gpgsign=false", "-c", "core.autocrlf=false"}
}

// shadowGit runs git on the session's checkpoint store with its folder as
// the work tree. Hooks, fsmonitor and the user's identity stay out.
func (e *Engine) shadowGit(ctx context.Context, host *environment.Local, sid string, timeout time.Duration, args ...string) (string, error) {
	return e.shadowGitWith(ctx, host, sid, timeout, nil, args...)
}

// shadowGitWith is shadowGit with more in git's environment.
func (e *Engine) shadowGitWith(ctx context.Context, host *environment.Local, sid string, timeout time.Duration, more map[string]string, args ...string) (string, error) {
	res, err := gitRunWith(ctx, host, timeout, more, append(e.shadowBase(sid, host), args...)...)
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(res.Output))
	}
	return strings.TrimSpace(res.Output), nil
}

// snapshotExclusions keeps .claude and .freeloop out of a snapshot. git add refuses a
// pathspec naming a folder that is ignored as a whole, even as an exclusion, though it
// adds everything else: that turned checkpoints off for a project ignoring .claude. So
// one ignored as a whole is left to the ignore rule, which nothing below it can undo.
// One ignored in part keeps its exclusion, since .claude/* with !.claude/commands/
// would bring files in. When git cannot say, the exclusions stay: at worst that is a
// snapshot failing, and never one holding these folders.
func (e *Engine) snapshotExclusions(ctx context.Context, host *environment.Local, sid string) []string {
	// Asked without a final slash: with one, .claude/* counts as ignoring the folder
	// itself, and its ! lines would then bring files in. Git sees for itself that it is
	// a folder, which is what .claude/ as a rule needs.
	folders := []string{".claude", ProjectDir}
	ignored := ""
	// check-ignore refuses to run with git's paths read literally, and these are the
	// app's own (see gitEnv).
	res, err := gitRunWith(ctx, host, 30*time.Second, gitPatterns, append(e.shadowBase(sid, host), append([]string{"check-ignore", "--"}, folders...)...)...)
	// It exits 0 with some ignored and 1 with none.
	if err == nil && res.ExitCode == 0 {
		ignored = res.Output
	}
	out := []string{}
	for _, f := range folders {
		if !slices.Contains(strings.Fields(ignored), f) {
			out = append(out, ":(exclude)"+f)
		}
	}
	return out
}

// snapshotHook, set by a test, runs in a snapshot before its files are added, as a
// long git add would.
var snapshotHook func(ctx context.Context, store string)

// snapshot commits the folder's current state and returns the commit.
func (e *Engine) snapshot(ctx context.Context, host *environment.Local, sid, message string) (string, error) {
	if !environment.IsDir(e.checkpointStore(sid)) {
		if _, err := gitRun(ctx, host, 30*time.Second, "init", "-q", "--bare", e.checkpointStore(sid)); err != nil {
			return "", err
		}
	}
	e.clearLocks(ctx, host, sid)
	// A nested repository stays out too. For one the index knows, git add
	// starts git in it to see whether it changed, under that repository's own
	// config, which can run programs and which a container can write. A
	// snapshot never held its files anyway, only the commit it stood at.
	if err := e.dropNested(ctx, host, sid); err != nil {
		return "", err
	}
	// The project's .claude/ is never written (a hard rule) and .freeloop/
	// holds the user's own settings and deny rules: snapshots leave both out,
	// so a rewind never changes them.
	if _, err := e.shadowGit(ctx, host, sid, checkpointTimeout, "rm", "-r", "-q", "--cached", "--ignore-unmatch", "--", ".claude", ProjectDir); err != nil {
		return "", err
	}
	if snapshotHook != nil {
		snapshotHook(ctx, e.checkpointStore(sid))
	}
	// The exclusions are pathspec magic, and every path here is the app's own, so this
	// add reads patterns (see gitEnv).
	if _, err := e.shadowGitWith(ctx, host, sid, checkpointTimeout, gitPatterns, append([]string{"add", "-A", "--", "."}, e.snapshotExclusions(ctx, host, sid)...)...); err != nil {
		return "", err
	}
	if err := e.dropNested(ctx, host, sid); err != nil {
		return "", err
	}
	if _, err := e.shadowGit(ctx, host, sid, 30*time.Second, "commit", "-q", "--allow-empty", "--no-verify", "-m", message); err != nil {
		return "", err
	}
	return e.shadowGit(ctx, host, sid, 10*time.Second, "rev-parse", "HEAD")
}

// clearLocks removes the lock files git leaves in the store when a snapshot is cut
// off, by Stop or by the app being killed: every snapshot after it would fail on
// them. A session's snapshots run one at a time, each holding the session busy, so a
// lock found as one starts is left over.
func (e *Engine) clearLocks(ctx context.Context, host *environment.Local, sid string) {
	store := e.checkpointStore(sid)
	locks := []string{"index.lock", "HEAD.lock"}
	if !slices.ContainsFunc(locks, func(l string) bool { return environment.IsFile(filepath.Join(store, l)) }) {
		return
	}
	// A commit cut off holds its branch's lock too.
	if ref, err := e.shadowGit(ctx, host, sid, 10*time.Second, "symbolic-ref", "-q", "HEAD"); err == nil && ref != "" {
		locks = append(locks, filepath.FromSlash(ref)+".lock")
	}
	for _, l := range locks {
		if err := environment.RemoveOwned(e.dataDir, filepath.Join(store, l)); err != nil {
			log.Printf("engine: clear the checkpoint store of %s: %v", sid, err)
		}
	}
}

// dropNested takes the nested repositories out of the store's index.
func (e *Engine) dropNested(ctx context.Context, host *environment.Local, sid string) error {
	ctx, cancel := context.WithTimeout(ctx, checkpointTimeout)
	defer cancel()
	store := "--git-dir=" + e.checkpointStore(sid)
	res, err := host.Exec(ctx, environment.ExecSpec{
		Argv: gitArgv(host, store, "--work-tree="+host.Root(), "ls-files", "--stage", "-z"),
		Env:  gitEnvFor(host, nil), MaxOutput: 64 << 20, Timeout: checkpointTimeout,
	})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("git ls-files: %s", strings.TrimSpace(res.Output))
	}
	var nested []string
	for _, entry := range strings.Split(res.Output, "\x00") {
		if mode, rest, ok := strings.Cut(entry, " "); ok && mode == "160000" {
			if _, p, ok := strings.Cut(rest, "\t"); ok {
				nested = append(nested, p)
			}
		}
	}
	for chunk := range slices.Chunk(nested, 200) {
		if _, err := e.shadowGit(ctx, host, sid, 30*time.Second, append([]string{"update-index", "--force-remove", "--"}, chunk...)...); err != nil {
			return err
		}
	}
	return nil
}

// checkpoint records the state before a prompt, under the turn's context: Stop
// cuts the snapshot off, and the caller sees ctx done. The session shows as running
// meanwhile, since a large folder takes a while. A failure turns checkpoints off for
// the runtime and says so once.
func (e *Engine) checkpoint(ctx context.Context, r *runtime) {
	if r.noCheckpoints {
		return
	}
	e.emit(r.sid, event.SessionStatus, event.SessionStatusData{Status: string(session.StatusRunning)})
	snap, cancel := context.WithTimeout(ctx, checkpointTimeout+time.Minute)
	defer cancel()
	commit, err := e.snapshot(snap, r.host, r.sid, "before a turn")
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		r.noCheckpoints = true
		log.Printf("engine: checkpoint %s: %v", r.sid, err)
		e.emit(r.sid, event.Notice, event.NoticeData{Message: "Checkpoints are off for this session: " + err.Error()})
		return
	}
	d := event.CheckpointData{Commit: commit}
	if h, ok := r.hs.(History); ok {
		d.HistoryLen = h.HistoryLen()
	}
	e.emit(r.sid, event.Checkpoint, d)
}

// RewindPreview says what a rewind to the checkpoint at seq would do beyond the
// session's own work. A checkpoint holds the whole folder, so other sessions working
// in it (no worktree of their own, subagents, profile switches) lose what they changed
// since: Others are those that ran a tool that may have changed files since then, and
// Busy names one in a turn, which holds the rewind back until its turn ends.
type RewindPreview struct {
	Others []event.SessionRef `json:"others"`
	Busy   string             `json:"busy,omitempty"`
}

// RewindPreview says what a rewind of sid to the checkpoint at seq would do to the
// other sessions in its folder.
func (e *Engine) RewindPreview(ctx context.Context, sid string, seq int64) (RewindPreview, error) {
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return RewindPreview{}, err
	}
	events, err := e.store.EventsLimit(ctx, sid, seq-1, 1)
	if err != nil {
		return RewindPreview{}, err
	}
	if len(events) != 1 || events[0].Seq != seq || events[0].Type != event.Checkpoint {
		return RewindPreview{}, errors.New("there is no checkpoint there")
	}
	p, _ := e.rewindPreview(ctx, ses, events[0].TS)
	return p, nil
}

// rewindPreview is RewindPreview from the checkpoint's time, with the other sessions
// in the folder.
func (e *Engine) rewindPreview(ctx context.Context, ses session.Session, since time.Time) (RewindPreview, []session.Session) {
	p := RewindPreview{Others: []event.SessionRef{}}
	sharing := e.sharingFolder(ctx, ses)
	for _, o := range sharing {
		if p.Busy == "" && e.turnRunning(o.ID) {
			p.Busy = o.Title
		}
		if e.changedFilesSince(ctx, o.ID, since) {
			p.Others = append(p.Others, event.SessionRef{SessionID: o.ID, Title: o.Title})
		}
	}
	return p, sharing
}

// sharingFolder lists the sessions other than ses that work in its folder, archived
// ones left out.
func (e *Engine) sharingFolder(ctx context.Context, ses session.Session) []session.Session {
	list, err := e.store.ListSessions(ctx)
	if err != nil {
		return nil
	}
	root := environment.Canonical(ses.WorkRoot())
	var out []session.Session
	for _, s := range list {
		if s.ID != ses.ID && s.Status != session.StatusArchived && environment.Canonical(s.WorkRoot()) == root {
			out = append(out, s)
		}
	}
	return out
}

// changedFilesSince reports whether a session ran a tool after since that may have
// changed files: one of any kind but reading, searching, thinking, fetching and
// switching modes, that started running (a call refused or cancelled before it ran
// changed nothing). It reads the transcript back to since.
func (e *Engine) changedFilesSince(ctx context.Context, sid string, since time.Time) bool {
	ran := map[string]bool{}
	changed := false
	e.store.ScanBack(ctx, sid, func(ev event.Event) bool {
		if !ev.TS.After(since) {
			// A call that ran after since but began before it: its kind is not
			// known here, so it counts.
			changed = len(ran) > 0
			return true
		}
		var d struct {
			ToolCallID string `json:"toolCallId"`
			Status     string `json:"status"`
			Kind       string `json:"kind"`
		}
		switch ev.Type {
		case event.ToolCallUpdate, event.ToolCall:
			json.Unmarshal(ev.Data, &d)
		default:
			return false
		}
		if d.Status == "in_progress" || d.Status == "completed" {
			ran[d.ToolCallID] = true
		}
		if ev.Type != event.ToolCall || !ran[d.ToolCallID] {
			return false
		}
		delete(ran, d.ToolCallID)
		switch permission.Kind(d.Kind) {
		case permission.KindRead, permission.KindSearch, permission.KindThink, permission.KindFetch, permission.KindSwitchMode:
			return false
		}
		changed = true
		return true
	})
	return changed
}

// Rewind restores the files of the checkpoint recorded at seq, after a
// snapshot of the current state, and cuts the built-in harness's context
// back to that point. Turns after it stay in the transcript, marked as
// rewound by the rewind event. The checkpoint holds the whole folder, so the
// rewind is refused while another session working in it is in a turn, and one
// that changed files since is told its files were rewound.
func (e *Engine) Rewind(ctx context.Context, sid string, seq int64) error {
	events, err := e.store.Events(ctx, sid, seq-1)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(events, func(ev event.Event) bool { return ev.Seq == seq && ev.Type == event.Checkpoint })
	if i < 0 {
		return errors.New("there is no checkpoint there")
	}
	var cp event.CheckpointData
	json.Unmarshal(events[i].Data, &cp)
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return err
	}
	preview, sharing := e.rewindPreview(ctx, ses, events[i].TS)
	if preview.Busy != "" {
		return errSharedTurn(preview.Busy)
	}
	// A compaction or an earlier rewind after the checkpoint replaced the
	// history, so the checkpoint's length no longer points into it.
	replaced := slices.ContainsFunc(events[i+1:], func(ev event.Event) bool {
		var h struct {
			Reset bool `json:"reset"`
		}
		return ev.Type == event.HistoryMessage && json.Unmarshal(ev.Data, &h) == nil && h.Reset
	})
	r, err := e.runtimeFor(ctx, sid)
	if err != nil {
		return err
	}
	// The session counts as busy while files are restored, so a prompt
	// sent meanwhile waits in the queue and starts afterwards. A turn that is
	// just ending gets a moment to finish, as for SwitchProfile: its end is
	// sent before the session is free.
	r.mu.Lock()
	for deadline := time.Now().Add(2 * time.Second); r.busy && !r.stopped; {
		done := r.done
		r.mu.Unlock()
		select {
		case <-done:
		case <-time.After(time.Until(deadline)):
			return errors.New("stop the running turn before rewinding")
		}
		r.mu.Lock()
	}
	if r.stopped {
		r.mu.Unlock()
		return errStopped
	}
	r.busy, r.done = true, make(chan struct{})
	r.mu.Unlock()
	defer e.release(r)
	saved, err := e.snapshot(ctx, r.host, sid, "before a rewind")
	if err != nil {
		return fmt.Errorf("save the current files: %w", err)
	}
	// A turn may have begun in the folder while the snapshot was taken.
	for _, o := range sharing {
		if e.turnRunning(o.ID) {
			return errSharedTurn(o.Title)
		}
	}
	if _, err := e.shadowGit(ctx, r.host, sid, checkpointTimeout, "read-tree", "--reset", "-u", cp.Commit); err != nil {
		return fmt.Errorf("restore the files: %w", err)
	}
	d := event.RewindData{CheckpointSeq: seq, Commit: cp.Commit, Saved: saved}
	if h, ok := r.hs.(History); ok && !replaced && cp.HistoryLen <= h.HistoryLen() {
		h.TruncateHistory(cp.HistoryLen)
		d.Context = true
	}
	e.emit(sid, event.Rewind, d)
	for _, o := range preview.Others {
		e.emit(o.SessionID, event.Notice, event.NoticeData{
			Message:   fmt.Sprintf("The session %q rewound the files of this folder to before one of its messages: changes this session made since may be undone.", ses.Title),
			RewoundBy: &event.SessionRef{SessionID: sid, Title: ses.Title},
		})
		e.gitChanged(o.SessionID)
	}
	e.gitChanged(sid)
	return nil
}

// errSharedTurn refuses a rewind while another session in the folder is in a turn.
func errSharedTurn(title string) error {
	return fmt.Errorf("the session %q is in a turn in the same folder, and a rewind would change its files under it: rewind when that turn ends, or stop it", title)
}
