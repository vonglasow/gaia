package filelock

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

type Mode int

const (
	Shared Mode = iota
	Exclusive
)

const retryInterval = 25 * time.Millisecond

// With acquires a lock on lockPath, executes fn, then releases the lock.
// If timeout is <= 0, it attempts lock acquisition only once.
func With(lockPath string, mode Mode, timeout time.Duration, fn func() error) error {
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open lock file %s: %w", lockPath, err)
	}
	defer func() {
		_ = lockFile.Close()
	}()

	if err := acquire(lockFile, mode, timeout); err != nil {
		return err
	}
	defer func() {
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
	}()

	return fn()
}

func acquire(lockFile *os.File, mode Mode, timeout time.Duration) error {
	lockType := syscall.LOCK_SH
	if mode == Exclusive {
		lockType = syscall.LOCK_EX
	}

	deadline := time.Now().Add(timeout)
	for {
		err := syscall.Flock(int(lockFile.Fd()), lockType|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			return fmt.Errorf("acquire file lock: %w", err)
		}

		if timeout <= 0 || time.Now().After(deadline) {
			return fmt.Errorf("timed out acquiring file lock after %s", timeout)
		}
		time.Sleep(retryInterval)
	}
}
