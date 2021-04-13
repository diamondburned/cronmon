package cronmon

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"os"
	"path/filepath"
	"time"
)

// Monitor is a cronmon instance that keeps a group of processes.
type Monitor struct {
	j Journaler

	ctx context.Context
	dir string

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

// RestoreMonitor tries to restore the previous monitor from the given
// journaler. If no previous monitor is found, then the monitor is started
// fresh.
func RestoreMonitor(ctx context.Context, dir string, j JournalReadWriter) *Monitor {
	m := newMonitor(ctx, dir, j)

	go func() {
		previous, err := ReadPreviousState(j)
		if err != nil {
			j.Write(&EventWarning{
				Component: "monitor",
				Error:     "error reading previous state, continuing as usual",
			})
			go m.RescanDir()
			return
		}

		files := m.readDir()

		m.sendFunc(func() {
			for _, f := range files {
				name := f.Name()
				pid, _ := previous.Processes[name]
				m.addFile(name, pid)
			}
		})
	}()

	return m
}

// NewMonitor creates a new monitor that oversees adding and removing
// processes. All files in the given directory will be scanned.
func NewMonitor(ctx context.Context, dir string, j Journaler) *Monitor {
	m := newMonitor(ctx, dir, j)
	go m.RescanDir()
	return m
}

func newMonitor(ctx context.Context, dir string, j Journaler) *Monitor {
	// Prepare the PPID directory.
	if err := os.MkdirAll(ppidPath(j), 0700); err != nil {
		j.Write(&EventWarning{
			Component: "monitor",
			Error:     "failed to mkdir -p ppidPath: " + err.Error(),
		})
	}

	m := &Monitor{
		j:     j,
		ctx:   ctx,
		dir:   dir,
		ctrl:  make(chan func()),
		watch: TryWatch(ctx, dir, j),
		procs: map[string]*Process{},
	}
	go m.monitor(ctx)

	return m
}

func ppidPath(j Journaler, tail ...string) string {
	// This could use some caching, but whatever.
	hash := md5.Sum([]byte(j.ID()))
	jwID := base64.RawStdEncoding.EncodeToString(hash[:])

	head := []string{os.TempDir(), "cronmon", jwID}
	head = append(head, tail...)

	return filepath.Join(head...)
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

// RescanDir rescans the directory for new files asynchronously.
func (m *Monitor) RescanDir() {
	go func() {
		files := m.readDir()
		if len(files) == 0 {
			return
		}

		m.sendFunc(func() {
			for _, file := range files {
				m.addFile(file.Name(), 0)
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

		case fn := <-m.ctrl:
			fn()

		case ev := <-m.watch.Events:
			switch ev.Op {
			case ProcessListAdd:
				m.addFile(ev.File, 0)
			case ProcessListRemove:
				m.removeFile(ev.File)
			case ProcessListUpdate:
				m.restartFile(ev.File)
			}
		}
	}
}

// addFile adds a new process with the given file into the store. If oldPID is
// 0, then the process is started, otherwise it is restored.
func (m *Monitor) addFile(file string, oldPID int) *Process {
	// Check that we haven't already added the file.
	p, ok := m.procs[file]
	if ok {
		return p
	}

	p = NewProcess(m.ctx, m.dir, file, m.j)
	m.procs[file] = p

	if oldPID == 0 {
		p.Start()
	} else {
		p.Takeover(oldPID)
	}

	return p
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

// restartFile restarts the process.
func (m *Monitor) restartFile(file string) {
	p, ok := m.procs[file]
	if ok {
		p.Stop()
		p.Start()
		return
	}

	m.j.Write(&EventWarning{
		Component: "cronmon",
		Error:     "attempted to restart non-existent process file " + file,
	})
}
