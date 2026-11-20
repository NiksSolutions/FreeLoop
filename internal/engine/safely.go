package engine

import (
	"log"
	"runtime/debug"
)

// A bug in the engine's background work must not take down the app and every session
// in it (NFR-REL-007), as promptSafely keeps one to its turn: each job runs safely, and
// a loop that runs for the app's life recovers each round and goes on, since a loop
// that ended would leave its work undone without a word.

// safely runs fn, and turns a panic in it into a log line with its stack.
func (e *Engine) safely(what string, fn func()) {
	defer func() {
		if v := recover(); v != nil {
			log.Printf("engine: %s panicked, and the app went on without it: %v\n%s", what, v, debug.Stack())
		}
	}()
	fn()
}

// goSafely runs fn safely as background work, which Close waits for.
func (e *Engine) goSafely(what string, fn func()) {
	e.bg.Go(func() { e.safely(what, fn) })
}
