package cronmon

import (
	"context"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"git.unix.lgbt/diamondburned/cronmon/cronmon/exec"
	"github.com/pkg/errors"
)

// ProcessWaitTimeout is the time to wait for a process to gracefully exit until
// forcefully terminating (and finally SIGKILLing) it.
var ProcessWaitTimeout = 3 * time.Second

// ProcessRetryBackoff is a list of backoff durations when a process fails to
// start. The last duration is used repetitively.
var ProcessRetryBackoff = []time.Duration{
	0,
	5 * time.Second,
	15 * time.Second,
	time.Minute,
}

// Process monitors an individual process. It is capable of self-monitoring the
// process, so any commanding operation simply cannot fail but only be delayed.
type Process struct {
	WaitTimeout  time.Duration
	RetryBackoff []time.Duration

	j Journaler

	ctx    context.Context
	cancel context.CancelFunc

	file string

	startCmd chan bool     // monitor, start command, true for restart
	exited   chan struct{} // process, process signal
	finalize chan error    // monitor, dead routine signal

	startProc func() (exec.Process, error)

	// states
	pmut sync.Mutex
	proc exec.Process
}

// NewProcess creates a new process and a background monitor. The process is
// terminated once the context times out. Wait must be called once the context
// is canceled to wait for the background routine to exit.
func NewProcess(ctx context.Context, dir, file string, j Journaler) *Process {
	ctx, cancel := context.WithCancel(ctx)
	arg0 := filepath.Join(dir, file)

	proc := &Process{
		WaitTimeout:  ProcessWaitTimeout,
		RetryBackoff: ProcessRetryBackoff,

		ctx:    ctx,
		cancel: cancel,

		j:        j,
		file:     file,
		startCmd: make(chan bool),
		exited:   make(chan struct{}, 1), // 1-buffered to hold in same routine
		finalize: make(chan error),

		startProc: func() (exec.Process, error) {
			return exec.StartProcess([]string{arg0})
		},
	}

	go proc.startMonitor()

	return proc
}

// Start starts a new process. If the process is already started, then it
// restarts the existing process.
func (proc *Process) Start(restart bool) {
	select {
	case <-proc.ctx.Done():
	case proc.startCmd <- restart:
	}
}

func (proc *Process) start(restart bool) {
	proc.pmut.Lock()

	if proc.proc != nil {
		if !restart {
			proc.pmut.Unlock()
			return
		}

		// Guarantee that the current process is stopped before spawning. This
		// prevents running two instances of the same process.
		proc.stop(false)
	}

	// Spawn a monitoring goroutine to report to proc.dead.
	go func() {
		// No matter the result of this goroutine, always mark the process as
		// dead for it to be restarted if needed.
		defer func() { proc.exited <- struct{}{} }()

		p, err := proc.startProc()
		if err != nil {
			proc.j.Write(&EventProcessSpawnError{
				File:   proc.file,
				Reason: err.Error(),
			})

			proc.pmut.Unlock()
			return
		}

		proc.proc = p
		proc.pmut.Unlock()

		proc.j.Write(&EventProcessSpawned{
			PID:  p.PID(),
			File: proc.file,
		})

		status := p.Wait()

		ev := EventProcessExited{
			File:     proc.file,
			PID:      status.PID,
			ExitCode: status.Code,
		}

		if status.Error != nil {
			ev.Error = status.Error.Error()
		}

		// Write to the journal before signaling that the process is dead to
		// ensure that the journal entry gets written.
		proc.j.Write(&ev)
	}()
}

// Stop stops the process permanently.
func (proc *Process) Stop() error {
	proc.cancel()
	return <-proc.finalize
}

func (proc *Process) stop(acquire bool) error {
	if acquire {
		proc.pmut.Lock()
		defer proc.pmut.Unlock()
	}

	if proc.proc == nil {
		// already stopped
		return nil
	}

	defer func() { proc.proc = nil }()

	if err := proc.proc.Signal(syscall.SIGTERM); err != nil {
		// Try to SIGKILL if we can't SIGTERM as a fallback.
		proc.proc.Kill()
	}

	after := time.NewTimer(proc.WaitTimeout)
	defer after.Stop()

	select {
	case <-after.C:
		proc.proc.Kill()
		<-proc.exited

		return errors.New("timed out waiting for program to exit")

	case <-proc.exited:
		return nil
	}
}

// startMonitor starts a monitoring routine that's in charge of restarting the
// process and handling incoming commands.
func (proc *Process) startMonitor() {
	var start <-chan time.Time // start backoff
	var timer *time.Timer
	var resetTime time.Time // deadline to consider app successfully started
	var restart bool

	backoff := -1 // backoff counter

	cleanupTimer := func() {
		if timer == nil {
			return
		}

		timer.Stop()
		timer = nil
		start = nil
	}

	for {
		select {
		case <-proc.ctx.Done():
			cleanupTimer()
			proc.finalize <- proc.stop(true)
			return

		case restart = <-proc.startCmd:
			start = dummyTimeCh()

		case <-start:
			proc.start(restart)
			restart = false
			cleanupTimer()

		case <-proc.exited:
			proc.proc = nil
			cleanupTimer()

			now := time.Now()

			// Check if we're past reset. If yes, then that means the process
			// has started successfully, so we can reset the backoff. If not,
			// then increment backoff and keep trying.
			if now.After(resetTime) {
				backoff = -1
			}

			startDura, resetDura := nextBackoff(proc.RetryBackoff, &backoff)
			resetTime = now.Add(resetDura)
			timer = time.NewTimer(startDura)
			start = timer.C
		}
	}
}

func dummyTimeCh() <-chan time.Time {
	ch := make(chan time.Time, 1)
	ch <- time.Time{}
	return ch
}

func nextBackoff(backoffs []time.Duration, ix *int) (start, reset time.Duration) {
	startIx := *ix
	resetIx := startIx

	if startIx < len(backoffs)-1 {
		startIx++
		resetIx++

		*ix = startIx

		if resetIx < len(backoffs)-2 {
			resetIx++
		}
	}

	return backoffs[startIx], backoffs[resetIx]
}
