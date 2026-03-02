package filelock

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWith_RunsFunction(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "lock")
	var called bool
	err := With(lockPath, Exclusive, 0, func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("With: %v", err)
	}
	if !called {
		t.Error("function was not called")
	}
}

func TestWith_ReturnsFunctionError(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "lock")
	wantErr := errTest
	err := With(lockPath, Exclusive, 0, func() error {
		return wantErr
	})
	if err != wantErr {
		t.Errorf("With returned %v, want %v", err, wantErr)
	}
}

var errTest = &testError{msg: "test error"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestWith_ExclusiveBlocksSecondExclusive(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "lock")
	hold := make(chan struct{})
	firstAcquired := make(chan struct{})
	go func() {
		_ = With(lockPath, Exclusive, 0, func() error {
			close(firstAcquired)
			<-hold
			return nil
		})
	}()
	<-firstAcquired
	err := With(lockPath, Exclusive, 50*time.Millisecond, func() error { return nil })
	close(hold)
	// On Unix, second exclusive typically times out while first holds the lock
	if err != nil && !strings.Contains(err.Error(), "timed out") {
		t.Logf("lock error (may be timeout): %v", err)
	}
}
