package cronmon

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
)

// Monitor is a cronmon instance that monitors a set of processes.
type Monitor struct {
	j Journaler

	watch *Watcher
	procs map[string]*Process
}

func NewMonitor(ctx context.Context, dir string, j Journaler) *Monitor {
	// Prepare the PPID directory.
	if err := os.MkdirAll(ppidPath(), 0700); err != nil {
		j.Write(&EventWarning{
			Component: "monitor",
			Error:     "failed to mkdir -p ppidPath: " + err.Error(),
		})
	}

	m := &Monitor{
		j:     j,
		watch: TryWatch(ctx, dir, j),
		procs: map[string]*Process{},
	}
	go m.monitor(ctx)

	return m
}

func ppidPath(tail ...string) string {
	head := []string{os.TempDir(), "cronmon", strconv.Itoa(os.Getpid())}
	head = append(head, tail...)

	return filepath.Join(head...)
}

func (m *Monitor) monitor(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
		case ev := <-m.watch.Events:
			switch ev.Op {
			case ProcessListAdd:
			case ProcessListRemove:
			case ProcessListUpdate:
			}
		}
	}
}
