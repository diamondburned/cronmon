package cronmon

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"git.unix.lgbt/diamondburned/cronmon/cronmon/exec"
	"github.com/pkg/errors"
)

// ProcessWaitTimeout is the time to wait for a process to gracefully exit until
// forcefully terminating (and finally SIGKILLing) it.
var ProcessWaitTimeout = time.Minute

// ProcessRetryBackoff is a list of backoff durations when a process fails to
// start. The last duration is used repetitively.
var ProcessRetryBackoff = []time.Duration{
	0,
	5 * time.Second,
	15 * time.Second,
	30 * time.Second,
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
	arg0 string

	evCh chan func()
	dead chan struct{}
	done chan error

	startProc func() (exec.Process, error)
	statusDir bool

	// states
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

		j:    j,
		file: file,
		arg0: arg0,
		evCh: make(chan func()),
		dead: make(chan struct{}, 1),
		done: make(chan error, 1),

		statusDir: true,
		startProc: func() (exec.Process, error) {
			return exec.StartProcess([]string{arg0})
		},
	}

	proc.startProc = proc.startProcess

	go proc.startMonitor()

	return proc
}

// startProcess reserves the PID file and starts the process.
func (proc *Process) startProcess() (exec.Process, error) {
	// Create the PID file if we have the status directory.
	path := ppidPath(proc.j, proc.file)

	// Only allow the child process to read from the file.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY, 0700)
	if err != nil {
		// This error isn't fatal, so we only warn.
		proc.j.Write(&EventWarning{
			Component: "process",
			Error:     "failed to make status file at " + path,
		})

		return exec.StartProcess([]string{proc.arg0})
	}

	// Spawn the process; skip the first 3 file descriptors, as they're stdin,
	// stdout and stderr.
	p, err := exec.StartProcessWithFiles([]string{proc.arg0}, []*os.File{nil, nil, nil, f})
	if err != nil {
		f.Close()
		os.Remove(path)
		return nil, err
	}

	// There's not much we can do if Remove fails.
	if err := os.Remove(path); err != nil {
		proc.j.Write(&EventWarning{
			Component: "process",
			Error:     "failed to remove status file: " + err.Error(),
		})
	}

	return p, nil
}

// Takeover takes over this process with the given PID if it was previously
// owned by the given PPID. The PPID should belong to a previous cronmon
// instance. If the process fails to be taken over, then it is started normally.
//
// If the process is already running, then Takeover does nothing, meaning it
// will prioritize its own started process.
func (proc *Process) Takeover(pid int) {
	select {
	case proc.evCh <- func() { proc.takeover(pid) }:
	case <-proc.ctx.Done():
	}
}

func (proc *Process) takeover(pid int) {
	if proc.proc != nil {
		return
	}

	statusFile := ppidPath(proc.j, proc.file)

	_, err := os.Stat(statusFile)
	if err != nil {
		proc.dead <- struct{}{}

		proc.j.Write(&EventProcessSpawnError{
			File:   proc.file,
			Reason: "status file " + statusFile + " not found",
		})
		return
	}

	p, err := exec.FindProcess(pid)
	if err != nil {
		// An error would never occur here on Linux. The error would go straight
		// into Wait, probably.
		proc.dead <- struct{}{}

		proc.j.Write(&EventWarning{
			Component: "process",
			Error:     "FindProcess error: " + err.Error(),
		})
		return
	}

	proc.proc = p
	proc.startWaiting(false)
}

// Start starts a new process. If the process is already started, then it does
// nothing.
func (proc *Process) Start() {
	select {
	case proc.evCh <- proc.start:
	case <-proc.ctx.Done():
	}
}

func (proc *Process) start() {
	if proc.proc != nil {
		return
	}

	p, err := proc.startProc()
	if err != nil {
		// Report that the process is dead so the monitor routine can restart
		// it.
		proc.dead <- struct{}{}

		proc.j.Write(&EventProcessSpawnError{
			File:   proc.file,
			Reason: err.Error(),
		})
		return
	}

	proc.proc = p
	proc.startWaiting(true)
}

// startWaiting reports the PID to the journal and starts a waiting routine.
func (proc *Process) startWaiting(createStatus bool) {
	// !!!: A critical failure might occur while this section is being executed:
	// if the PID is not written into the journal in time, then the new cronmon
	// process won't be aware of the running process. There's not really a way
	// around this.

	proc.j.Write(&EventProcessSpawned{
		PID:  proc.proc.PID(),
		File: proc.file,
	})

	// Spawn a monitoring goroutine to report to proc.dead.
	go func() {
		status := proc.proc.Wait()

		ev := EventProcessExited{
			PID:      status.PID,
			File:     proc.file,
			ExitCode: status.Code,
		}

		if status.Error != nil {
			ev.Error = status.Error.Error()
		}

		// Write to the journal before signaling that the process is dead to
		// ensure that the journal entry gets written.
		proc.j.Write(&ev)

		proc.dead <- struct{}{}
	}()
}

// Stop stops the process, if it's running. An error is returned if it's not
// running.
func (proc *Process) Stop() error {
	proc.cancel()
	return <-proc.done
}

func (proc *Process) stop() error {
	if proc.proc == nil {
		// already stopped
		return nil
	}

	if err := proc.proc.Signal(os.Interrupt); err != nil {
		// Try to SIGKILL if we can't SIGINT (looking at you, Windows).
		proc.proc.Kill()
	}

	after := time.NewTimer(proc.WaitTimeout)
	defer after.Stop()

	for {
		select {
		case <-after.C:
			// Timeout reached and the program still hasn't exited yet. Send
			// SIGKILL and bail, since there's not much we can do here.
			proc.proc.Kill()

			// Wait until the process routine exits.
			<-proc.dead

			return errors.New("timed out waiting for program to exit")

		case <-proc.dead:
			return nil
		}
	}
}

// startMonitor starts a monitoring routine that's in charge of restarting the
// process and handling incoming commands.
func (proc *Process) startMonitor() {
	var start <-chan time.Time // start backoff
	var timer *time.Timer
	var resetTime time.Time // deadline to consider app successfully started

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
			proc.done <- proc.stop()
			cleanupTimer()
			return

		case <-start:
			proc.start()
			cleanupTimer()

		case <-proc.dead:
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

		case fn := <-proc.evCh:
			fn()
		}
	}
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
