package cronmon

import (
	"io"
	"time"

	"github.com/pkg/errors"
)

// Journaler describes a journal writer/logger.
type Journaler interface {
	// ID returns the ID of the journaler.
	ID() string
	// Write writes the event into the journaler.
	Write(Event) error
}

// JournalReader describes a journal reader.
type JournalReader interface {
	Read() (Event, time.Time, error)
}

// JournalReadWriter is a journal reader and writer.
type JournalReadWriter interface {
	Journaler
	JournalReader
}

// ReadPreviousState reads from the JournalReader the previous state of the
// cronmon monitor.
func ReadPreviousState(r JournalReader) (*PreviousState, error) {
	state := PreviousState{
		Processes: map[string]int{},
	}
	hasQuit := false
	deleted := map[int]struct{}{}

	for {
		event, time, err := r.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, io.ErrUnexpectedEOF
			}

			return nil, err
		}

		switch data := event.(type) {
		case *EventAcquired:
			state.StartedAt = time
			return &state, nil

		case *EventQuit:
			hasQuit = true

		case *EventProcessExited:
			deleted[data.PID] = struct{}{}

		case *EventProcessSpawned:
			if !hasQuit {
				// If the process is still alive, then it shouldn't be in the
				// deleted map, since it'll appear later.
				if _, ok := deleted[data.PID]; !ok && !hasQuit {
					state.Processes[data.File] = data.PID
				}
			}
		}
	}
}
