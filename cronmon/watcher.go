package cronmon

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
)

// Watcher is a cronmon watcher that watches the configuration directory
// for new processes.
type Watcher struct {
	Events chan EventProcessListModify

	w   *fsnotify.Watcher
	j   Journaler
	dir string
}

// TryWatch attempts to watch the given directory asynchronously, but it will
// log into the journaler if, for some reason, it fails to watch the directory.
func TryWatch(ctx context.Context, dir string, j Journaler) *Watcher {
	w := newWatcher(dir, j)

	go func() {
		if err := w.init(); err != nil {
			j.Write(&EventWarning{
				Component: "watcher",
				Error:     fmt.Sprintf("not watching dir because: %v", err),
			})
			return
		}

		w.watch(ctx)
	}()

	return w
}

// Watch watches the given directory and logs events into the journaler.
// The watcher is stopped once the given context is canceled.
func NewWatcher(ctx context.Context, dir string, j Journaler) (*Watcher, error) {
	w := newWatcher(dir, j)
	if err := w.init(); err != nil {
		return nil, err
	}

	go w.watch(ctx)
	return w, nil
}

func newWatcher(dir string, j Journaler) *Watcher {
	return &Watcher{
		Events: make(chan EventProcessListModify),
		w:      nil,
		j:      j,
		dir:    dir,
	}
}

func (w *Watcher) init() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.Wrap(err, "failed to create watcher")
	}

	if err := watcher.Add(w.dir); err != nil {
		return errors.Wrap(err, "failed to watch dir")
	}

	w.w = watcher
	return nil
}

func (w *Watcher) watch(ctx context.Context) {
	defer w.w.Close()

	for {
		select {
		case <-ctx.Done():
			return

		case err := <-w.w.Errors:
			w.j.Write(&EventWarning{
				Component: "watcher",
				Error:     "inotify error: " + err.Error(),
			})

		case evt := <-w.w.Events:
			events := translateFsnotifyEvt(evt, w.dir)
			if len(events) == 0 {
				w.j.Write(&EventWarning{
					Component: "watcher",
					Error:     fmt.Sprintf("skipped unknown %s event at %q", evt.Op, evt.Name),
				})

				continue
			}

			for _, event := range events {
				select {
				case w.Events <- event:
					continue
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

// translateFsnotifyEvt translates an fsnotify event into a list of
// EventProcessListModify events.
func translateFsnotifyEvt(evt fsnotify.Event, dir string) []EventProcessListModify {
	evDir, name := filepath.Split(dir)
	if evDir != dir {
		return nil
	}

	switch {
	case evt.Op&fsnotify.Write != 0:
		return []EventProcessListModify{
			{Op: ProcessListUpdate, File: name},
		}
	case evt.Op&fsnotify.Create != 0:
		return []EventProcessListModify{
			{Op: ProcessListAdd, File: name},
		}
	case evt.Op&fsnotify.Rename != 0:
		// Treat a rename as a remove; fsnotify does not report renames
		// properly, so it's apparently treated like a remove.
		// See: https://github.com/fsnotify/fsnotify/issues/26

		fallthrough
	case evt.Op&fsnotify.Remove != 0:
		return []EventProcessListModify{
			{Op: ProcessListRemove, File: name},
		}
	}

	return nil
}
