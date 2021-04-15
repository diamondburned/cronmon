package cronmon

import (
	"context"
	"os"
	"time"

	"github.com/pkg/errors"
)

// Monitor is a cronmon instance that keeps a group of processes.
type Monitor struct {
	j Journaler

	ctx    context.Context
	cancel context.CancelFunc

	dir   string
	done  chan struct{}
	ctrl  chan func()
	procs map[string]*Process
	watch *Watcher
}

// PreviousState parses the last cronmon's previous state to be used by Monitor
// for restoring.
type PreviousState struct {
	StartedAt time.Time
	// Processes contains a map of known files to the previous PIDs.
	Processes map[string]int
}

// NewMonitor creates a new monitor that oversees adding and removing processes.
// All files in the given directory will be scanned.
func NewMonitor(ctx context.Context, dir string, j Journaler) (*Monitor, error) {
	m, err := newMonitor(ctx, dir, j)
	if err != nil {
		return nil, err
	}

	j.Write(&EventAcquired{
		JournalID: j.ID(),
	})

	m.RescanDir()
	return m, nil
}

func newMonitor(ctx context.Context, dir string, j Journaler) (*Monitor, error) {
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, errors.Wrap(err, "failed to create scripts directory")
	}

	ctx, cancel := context.WithCancel(ctx)

	m := &Monitor{
		j:      j,
		ctx:    ctx,
		cancel: cancel,
		dir:    dir,
		done:   make(chan struct{}),
		ctrl:   make(chan func()),
		watch:  TryWatch(ctx, dir, j),
		procs:  map[string]*Process{},
	}
	go m.monitor(ctx)

	return m, nil
}

func (m *Monitor) readDir() []os.DirEntry {
	files, err := os.ReadDir(m.dir)
	if err != nil {
		m.j.Write(&EventWarning{
			Component: "monitor",
			Error:     "failed to scan directory: " + err.Error(),
		})
	}
	return files
}

// Stop stops all processes as well as the main monitoring loop then wait for
// all processes to end and for the monitoring routine to die.
func (m *Monitor) Stop() {
	// Cancelling this context will interrupt all programs in the background.
	m.cancel()
	// Ensure the control routine has exited so we can end everything in this
	// routine instead.
	<-m.done

	// Ensure that all processes are fully stopped.
	for _, proc := range m.procs {
		proc.Stop()
	}

	m.j.Write(&EventQuit{})
}

// RescanDir rescans the directory for new files asynchronously.
func (m *Monitor) RescanDir() {
	go func() {
		files := m.readDir()
		if len(files) == 0 {
			return
		}

		m.sendFunc(func() {
			for _, file := range files {
				m.addFile(file.Name(), false)
			}
		})
	}()
}

func (m *Monitor) sendFunc(fn func()) {
	select {
	case m.ctrl <- fn:
	case <-m.ctx.Done():
	}
}

func (m *Monitor) monitor(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			m.done <- struct{}{}

		case fn := <-m.ctrl:
			fn()

		case ev := <-m.watch.Events:
			switch ev.Op {
			case ProcessListAdd:
				m.addFile(ev.File, false)
			case ProcessListUpdate:
				m.addFile(ev.File, true)
			case ProcessListRemove:
				m.removeFile(ev.File)
			}
		}
	}
}

// addFile adds a new process with the given file into the store. If oldPID is
// 0, then the process is started, otherwise it is restored.
func (m *Monitor) addFile(file string, restart bool) *Process {
	// Check that we haven't already added the file.
	pr, ok := m.procs[file]
	if !ok {
		pr = NewProcess(m.ctx, m.dir, file, m.j)
		m.procs[file] = pr
	}

	pr.Start(restart)
	return pr
}

// removeFile removes a process with the given file name. The process is
// stopped.
func (m *Monitor) removeFile(file string) {
	p, ok := m.procs[file]
	if ok {
		p.Stop()
		delete(m.procs, file)
		return
	}

	m.j.Write(&EventWarning{
		Component: "cronmon",
		Error:     "attempted to remove non-existent process file " + file,
	})
}
