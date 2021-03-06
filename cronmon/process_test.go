package cronmon

import (
	"context"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"git.unix.lgbt/diamondburned/cronmon/cronmon/exec"
	"github.com/pkg/errors"
)

const forever time.Duration = math.MaxInt64

func TestProcess(t *testing.T) {
	t.Run("graceful interrupt", func(t *testing.T) {
		nextPID := newNextPID()
		var j mockJournal

		proc := NewProcess(context.Background(), "", "sleep", &j)
		proc.RetryBackoff = []time.Duration{0} // no backoff
		proc.startProc = func() (exec.Process, error) {
			return exec.NewSleepProcess(forever, 0, nextPID()), nil
		}
		proc.Start(false)

		// Stop guarantees that the background routines would've been exited by
		// the time the function returns.
		if err := proc.Stop(); err != nil {
			t.Error("failed to stop process:", err)
		}

		j.Verify(t, true, []Event{
			&EventProcessSpawned{PID: 1, File: "sleep"},
			&EventProcessExited{PID: 1, File: "sleep", ExitCode: 0},
		})
	})

	t.Run("kill timeout", func(t *testing.T) {
		nextPID := newNextPID()
		var j mockJournal

		proc := NewProcess(context.Background(), "", "sleep", &j)
		proc.WaitTimeout = time.Microsecond
		proc.RetryBackoff = []time.Duration{0} // no backoff
		proc.startProc = func() (exec.Process, error) {
			return exec.NewSleepProcess(forever, forever, nextPID()), nil
		}
		proc.Start(false)
		// Ignore the error since we can check the journal.
		proc.Stop()

		j.Verify(t, true, []Event{
			&EventProcessSpawned{PID: 1, File: "sleep"},
			&EventProcessExited{PID: 1, File: "sleep", ExitCode: -1},
		})
	})

	t.Run("backoff", func(t *testing.T) {
		var j mockJournal

		var attempts uint32

		proc := NewProcess(context.Background(), "", "sleep", &j)
		proc.RetryBackoff = []time.Duration{
			0,
			1 * time.Microsecond,
			5 * time.Microsecond,
			time.Second,
		}
		proc.startProc = func() (exec.Process, error) {
			attempt := atomic.AddUint32(&attempts, 1)
			if attempt > 3 {
				return nil, errors.New("after")
			}
			return nil, errors.New("before")
		}
		proc.Start(false)

		time.Sleep(time.Millisecond / 2)

		if err := proc.Stop(); err != nil {
			t.Error("failed to stop process:", err)
		}

		j.Finalize()
		j.Verify(t, false, []Event{
			&EventProcessSpawnError{File: "sleep", Reason: "before"},
			&EventProcessSpawnError{File: "sleep", Reason: "before"},
			&EventProcessSpawnError{File: "sleep", Reason: "before"},
			&EventProcessSpawnError{File: "sleep", Reason: "after"},
		})
	})

	t.Run("autorestart", func(t *testing.T) {
		nextPID := newNextPID()
		var j mockJournal

		newProcCh := make(chan struct{})

		proc := NewProcess(context.Background(), "", "sleep", &j)
		proc.RetryBackoff = []time.Duration{0} // no backoff
		proc.startProc = func() (exec.Process, error) {
			select {
			case newProcCh <- struct{}{}:
			default:
			}
			return exec.NewSleepProcess(0, 0, nextPID()), nil
		}
		proc.Start(false)

		var count int
		for range newProcCh {
			count++
			if count > 5 {
				break
			}
		}

		if err := proc.Stop(); err != nil {
			t.Error("failed to stop process:", err)
		}

		expect := make([]Event, 0, 10)
		for i := 0; i < 5; i++ {
			expect = append(expect,
				&EventProcessSpawned{PID: i + 1, File: "sleep"},
				&EventProcessExited{PID: i + 1, File: "sleep", ExitCode: 0},
			)
		}

		j.Finalize()
		remaining := j.Verify(t, false, expect)
		t.Log("remaining journals:", remaining)
	})
}

func newNextPID() func() int {
	var pid uint32
	return func() int { return int(atomic.AddUint32(&pid, 1)) }
}
