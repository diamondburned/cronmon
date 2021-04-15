// Package exec provides an abstraction around package os' Process
// implementation for easier testing.
package exec

import (
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

// Process describes a command process.
type Process interface {
	PID() int
	Signal(os.Signal) error
	Kill() error
	Wait() ExitStatus
}

// ExitStatus is a process' exit status.
type ExitStatus struct {
	PID   int
	Code  int // -1 for interrupt
	Error error
}

type process struct {
	*os.Process
}

var _ Process = process{}

// FindProcess creates a new Process from an existing process ID.
func FindProcess(pid int) (Process, error) {
	p, err := os.FindProcess(pid)
	if err != nil {
		return nil, err
	}

	return process{p}, nil
}

// StartProcess creates a new command process on the system.
func StartProcess(argv []string) (Process, error) {
	// Lock this goroutine to the OS thread for Pdeathsig.
	// See https://github.com/golang/go/issues/27505.
	runtime.LockOSThread()

	// Linux-only: we need to set the current PID as the subreaper to prevent
	// the processes we're spawning from disowning itself, because we might
	// accidentally spawn multiple instances of it while thinking it's dead.
	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
		return nil, errors.Wrap(err, "failed to set subreaper")
	}

	p, err := os.StartProcess(argv[0], argv, &os.ProcAttr{
		// Linux-only: we need the child to die when we do, because it's the
		// next best thing we can do that doesn't involve reparenting orphaned
		// children magic.
		Sys: &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM},
	})
	if err != nil {
		return nil, err
	}

	return process{p}, nil
}

func (proc process) PID() int {
	return proc.Pid
}

// Wait waits for the process to exit. It must be called on the same goroutine
// as StartProcess.
func (proc process) Wait() ExitStatus {
	s, err := proc.Process.Wait()
	runtime.UnlockOSThread()

	return ExitStatus{
		PID:   proc.Pid,
		Code:  s.ExitCode(),
		Error: err,
	}
}

type sleepProcess struct {
	once  sync.Once
	stop  chan struct{}
	timer *time.Timer
	delay time.Duration

	pid  int
	exit int32
}

// NewSleepProcess creates a process that only idles for a duration. It is used
// for testing. If delay is larger than 0, then the process will sleep for that
// delay before exiting, unless it is SIGKILLed.
func NewSleepProcess(dura, delay time.Duration, pid int) Process {
	return &sleepProcess{
		stop:  make(chan struct{}),
		timer: time.NewTimer(dura),
		delay: delay,

		pid:  pid,
		exit: -2,
	}
}

func (mock *sleepProcess) PID() int { return mock.pid }

func (mock *sleepProcess) Signal(sig os.Signal) error {
	var status int32

	switch sig {
	case syscall.SIGINT, syscall.SIGTERM: // catchable
		status = 0
	case syscall.SIGKILL:
		status = -1
	default:
		return errors.New("unknown signal")
	}

	go func() {
		if mock.delay > 0 && sig != os.Kill {
			select {
			case <-time.After(mock.delay):

			case <-mock.stop:
				return
			}
		}

		// Ensure exit is still unset (-2), otherwise bail.
		if !atomic.CompareAndSwapInt32(&mock.exit, -2, status) {
			return
		}

		close(mock.stop)
		mock.timer.Stop()
	}()

	return nil
}

func (mock *sleepProcess) Kill() error {
	return mock.Signal(os.Kill)
}

func (mock *sleepProcess) Wait() ExitStatus {
	mock.once.Do(func() {
		select {
		case <-mock.stop:
		case <-mock.timer.C:
			atomic.StoreInt32(&mock.exit, 0)
		}
	})

	return ExitStatus{
		PID:  mock.pid,
		Code: int(atomic.LoadInt32(&mock.exit)),
	}
}
