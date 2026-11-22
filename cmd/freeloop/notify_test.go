//go:build !server

package main

import (
	"errors"
	"testing"
)

// fakePermission is the system's notification permission as the user sets it.
type fakePermission struct {
	allowed          bool
	requests, checks int
}

func (f *fakePermission) RequestNotificationAuthorization() (bool, error) {
	f.requests++
	if !f.allowed {
		return false, errors.New("Notifications are not allowed for this application")
	}
	return true, nil
}

func (f *fakePermission) CheckNotificationAuthorization() (bool, error) {
	f.checks++
	return f.allowed, nil
}

// GUI-008: a refusal is not kept for the rest of the run. On 2026-11-22 the user
// allowed notifications in System Settings after the first notice was refused, and
// none came until the app was restarted.
func TestNotificationsAllowedLaterTakeEffect(t *testing.T) {
	var g notifyGate
	p := &fakePermission{}
	if g.allowed(p) {
		t.Fatal("a refused permission let a notice through")
	}
	if p.requests != 1 {
		t.Fatalf("the first notice asks once: %d requests", p.requests)
	}
	if g.allowed(p) {
		t.Fatal("still refused, a notice got through")
	}
	if p.requests != 1 || p.checks != 1 {
		t.Errorf("a later notice checks again without asking again: %d requests, %d checks", p.requests, p.checks)
	}
	// The user allows notifications in System Settings.
	p.allowed = true
	if !g.allowed(p) {
		t.Fatal("allowed in System Settings, the next notice was still dropped")
	}
	// Once allowed, notices go without asking the system each time.
	checks := p.checks
	g.allowed(p)
	if p.checks != checks || p.requests != 1 {
		t.Errorf("an allowed permission was asked about again: %d requests, %d checks", p.requests, p.checks-checks)
	}
	// Turned off again, a failed send makes the next notice check again.
	p.allowed = false
	g.failed(errors.New("Notifications are not allowed for this application"))
	if g.allowed(p) {
		t.Error("turned off in System Settings, a notice got through")
	}
}
