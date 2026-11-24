package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// failQueueSaves makes the engine's saves of the queue fail while fail says so.
func failQueueSaves(e *Engine, fail func(q config.Queue) bool) {
	e.queueMu.Lock()
	defer e.queueMu.Unlock()
	e.queueSave = func(dir string, q config.Queue) error {
		if fail(q) {
			return errors.New("disk full")
		}
		return config.SaveQueue(dir, q)
	}
}

// started says a save records a task's start: running, and no longer starting.
func started(q config.Queue) bool {
	for _, t := range q.Tasks {
		if t.State == config.TaskRunning && !t.Starting {
			return true
		}
	}
	return false
}

func quoteJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func sessionCount(t *testing.T, e *Engine) int {
	t.Helper()
	list, err := e.store.ListSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return len(list)
}

// SES-025: a task is saved as starting, with its session's ID, before the session starts,
// and the session starts outside queueMu.
func TestQueueTaskIsSavedAsStartingFirst(t *testing.T) {
	ctx := context.Background()
	e, root := queueEngine(t)
	if _, err := e.AddTask(config.Task{Prompt: "first", ProjectRoot: root, ProfileID: "main"}); err != nil {
		t.Fatal(err)
	}
	var seen int32
	e.taskStarting = func(task config.Task) {
		atomic.AddInt32(&seen, 1)
		if !e.queueMu.TryLock() {
			t.Error("queueMu is held while a session starts")
		} else {
			e.queueMu.Unlock()
		}
		q, err := config.LoadQueue(e.dataDir)
		if err != nil || len(q.Tasks) != 1 || !q.Tasks[0].Starting || q.Tasks[0].State != config.TaskRunning || q.Tasks[0].SessionID == "" {
			t.Errorf("saved before the start: %+v, %v", q.Tasks, err)
		}
		if n := sessionCount(t, e); n != 0 {
			t.Errorf("%d sessions before the start", n)
		}
	}
	e.runQueue(ctx, time.Now())
	if atomic.LoadInt32(&seen) != 1 {
		t.Fatalf("started %d tasks", seen)
	}
	q, _ := e.Queue()
	if q.Tasks[0].State != config.TaskRunning || q.Tasks[0].Starting {
		t.Errorf("after the start: %+v", q.Tasks[0])
	}
	if _, err := e.store.GetSession(ctx, q.Tasks[0].SessionID); err != nil {
		t.Errorf("the task's session is not the one it saved: %v", err)
	}
}

// SES-025: a save that fails after a task's session started does not start it again on
// the next check, which finds the session and records it.
func TestQueueTaskStartsOnceWhenTheSaveAfterFails(t *testing.T) {
	ctx := context.Background()
	e, root := queueEngine(t)
	if _, err := e.AddTask(config.Task{Prompt: "first", ProjectRoot: root, ProfileID: "main"}); err != nil {
		t.Fatal(err)
	}
	var failed int32
	failQueueSaves(e, func(q config.Queue) bool {
		return started(q) && atomic.CompareAndSwapInt32(&failed, 0, 1)
	})
	e.runQueue(ctx, time.Now())
	if atomic.LoadInt32(&failed) != 1 {
		t.Fatal("no save of the start failed")
	}
	e.runQueue(ctx, time.Now())
	e.runQueue(ctx, time.Now())
	if n := sessionCount(t, e); n != 1 {
		t.Fatalf("%d sessions for one task", n)
	}
	list, _ := e.store.ListSessions(ctx)
	q, _ := e.Queue()
	if q.Tasks[0].SessionID != list[0].ID || q.Tasks[0].Starting || q.Tasks[0].State == config.TaskWaiting {
		t.Errorf("task = %+v, session %s", q.Tasks[0], list[0].ID)
	}
}

// SES-025: the app stopping between a task's start and the save of it leaves one session:
// the next start of the app finds it. A task saved as starting whose session was never
// made waits again, and starts once.
func TestQueueTaskStartedBeforeACrash(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	root := t.TempDir()
	e := openQueueEngine(t, dir)
	if _, err := e.AddTask(config.Task{Prompt: "first", ProjectRoot: root, ProfileID: "main"}); err != nil {
		t.Fatal(err)
	}
	ch, unsub := e.Subscribe()
	// The save after the start is what the crash cuts off.
	failQueueSaves(e, started)
	e.runQueue(ctx, time.Now())
	list, _ := e.store.ListSessions(ctx)
	if len(list) != 1 {
		t.Fatalf("%d sessions", len(list))
	}
	sid := list[0].ID
	waitIdle(t, ch, sid)
	unsub()
	// A crash: nothing is closed; a new engine opens the folder.
	e.mu.Lock()
	e.runtimes = map[string]*runtime{}
	e.mu.Unlock()
	e.bus.close()
	e.store.Close()

	e = openQueueEngine(t, dir)
	t.Cleanup(func() { e.Close() })
	e.runQueue(ctx, time.Now())
	e.runQueue(ctx, time.Now())
	if n := sessionCount(t, e); n != 1 {
		t.Fatalf("%d sessions after the crash, want 1", n)
	}
	q, _ := e.Queue()
	if q.Tasks[0].SessionID != sid || q.Tasks[0].Starting || q.Tasks[0].State == config.TaskWaiting {
		t.Errorf("task = %+v", q.Tasks[0])
	}

	// Saved as starting, and the app stopped before the session was made.
	q.Tasks = append(q.Tasks, config.Task{ID: "task_2", Title: "second", Prompt: "second", ProjectRoot: root, ProfileID: "main",
		State: config.TaskRunning, Starting: true, SessionID: session.NewID(time.Now()), AddedAt: time.Now().UTC(), StartedAt: time.Now().UTC()})
	if err := config.SaveQueue(dir, q); err != nil {
		t.Fatal(err)
	}
	e.runQueue(ctx, time.Now())
	e.runQueue(ctx, time.Now())
	if n := sessionCount(t, e); n != 2 {
		t.Fatalf("%d sessions, want 2", n)
	}
	q, _ = e.Queue()
	if t2 := q.Tasks[1]; t2.Starting || t2.State == config.TaskWaiting {
		t.Errorf("second task = %+v", t2)
	} else if _, err := e.store.GetSession(ctx, t2.SessionID); err != nil {
		t.Errorf("the second task's session: %v", err)
	}
}

// SES-025: a queue.json the released beta wrote, which knows nothing of starting, loads
// and moves on as before.
func TestQueueFromTheBetaLoads(t *testing.T) {
	ctx := context.Background()
	e, root := queueEngine(t)
	ses, err := e.CreateSession(ctx, CreateSessionRequest{Title: "earlier", ProjectRoot: root, ProfileID: "main"})
	if err != nil {
		t.Fatal(err)
	}
	beta := `{
  "paused": false,
  "limit": 2,
  "tasks": [
    {"id": "task_1", "title": "earlier", "prompt": "earlier", "projectRoot": ` + quoteJSON(root) + `, "profileId": "main", "worktree": false,
     "state": "running", "addedAt": "2026-11-20T10:00:00Z", "sessionId": "` + ses.ID + `", "startedAt": "2026-11-20T10:00:01Z"},
    {"id": "task_2", "title": "next", "prompt": "next", "projectRoot": ` + quoteJSON(root) + `, "profileId": "main", "worktree": false,
     "state": "waiting", "addedAt": "2026-11-20T10:00:00Z", "startedAt": "0001-01-01T00:00:00Z"}
  ]
}`
	if err := os.WriteFile(filepath.Join(e.dataDir, config.QueueFile), []byte(beta), 0o600); err != nil {
		t.Fatal(err)
	}
	q, err := e.Queue()
	if err != nil || len(q.Tasks) != 2 || q.Tasks[0].Starting {
		t.Fatalf("queue = %+v, %v", q, err)
	}
	e.runQueue(ctx, time.Now())
	q, _ = e.Queue()
	if q.Tasks[0].SessionID != ses.ID || q.Tasks[0].State != config.TaskRunning || q.Tasks[1].State != config.TaskRunning || q.Tasks[1].Starting {
		t.Errorf("after a check: %+v", q.Tasks)
	}
	if n := sessionCount(t, e); n != 2 {
		t.Errorf("%d sessions, want the earlier one and the next task's", n)
	}
}
