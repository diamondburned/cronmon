package cronmon

import (
	"encoding/json"
	"fmt"
)

// eventType describes an event type.
type eventType = string

const (
	eventWarning           eventType = "warning"
	eventAcquired          eventType = "acquired lock"
	eventLogTruncated      eventType = "log truncated"
	eventProcessSpawnError eventType = "process spawn error"
	eventProcessSpawned    eventType = "process spawned"
	eventProcessExited     eventType = "process exited"
	eventProcessListModify eventType = "process list modified"
)

// Event is an interface describing known events.
type Event interface {
	Type() string
	event()
}

var eventFuncs = map[eventType]func() Event{
	eventWarning:           func() Event { return &EventWarning{} },
	eventAcquired:          func() Event { return &EventAcquired{} },
	eventLogTruncated:      func() Event { return &EventLogTruncated{} },
	eventProcessSpawnError: func() Event { return &EventProcessSpawnError{} },
	eventProcessSpawned:    func() Event { return &EventProcessSpawned{} },
	eventProcessExited:     func() Event { return &EventProcessExited{} },
	eventProcessListModify: func() Event { return &EventProcessListModify{} },
}

// UnmarshalEventJSON tries to unmarshal an event from the given JSON bytes.
func UnmarshalEventJSON(typ string, data []byte) (Event, error) {
	eventFunc := eventFuncs[eventType(typ)]
	if eventFunc == nil {
		return nil, fmt.Errorf("unknown event type %q", typ)
	}

	event := eventFunc()
	return event, json.Unmarshal(data, event)
}

// EventWarning is emitted when a non-fatal error occurs.
type EventWarning struct {
	Component string `json:"component"`
	Error     string `json:"error"`
}

func (ev EventWarning) Type() string { return eventWarning }
func (ev EventWarning) event()       {}

// EventAcquired is emitted when the flock (i.e. write lock on the journal) is
// acquired, which is on startup.
type EventAcquired struct {
	// PPID is cronmon's process ID.
	PPID int `json:"ppid"`
}

func (ev EventAcquired) Type() string { return eventAcquired }
func (ev EventAcquired) event()       {}

// EventLogTruncated is emitted when the log file has been truncated for any
// reason, including a corrupted log file.
type EventLogTruncated struct {
	Reason string `json:"reason"`
}

func (ev EventLogTruncated) Type() string { return eventLogTruncated }
func (ev EventLogTruncated) event()       {}

// EventProcessSpawnError is emitted when a process fails to start for any
// reason.
type EventProcessSpawnError struct {
	File   string `json:"file"`
	Reason string `json:"reason"`
}

func (ev EventProcessSpawnError) Type() string { return eventProcessSpawnError }
func (ev EventProcessSpawnError) event()       {}

// EventProcessSpawned is emitted when a process has been started for any
// reason.
type EventProcessSpawned struct {
	PID  int    `json:"pid"`
	File string `json:"file"`
}

func (ev EventProcessSpawned) Type() string { return eventProcessSpawned }
func (ev EventProcessSpawned) event()       {}

// EventProcessExited is emitted when a process has been stopped for any reason.
type EventProcessExited struct {
	PID      int    `json:"pid"`
	File     string `json:"file"`
	Error    string `json:"error,omitempty"`
	ExitCode int    `json:"exit_code"` // -1 if interrupted or terminated
}

// IsGraceful returns true if the process stopped gracefully (i.e. on SIGINT).
func (ev EventProcessExited) IsGraceful() bool {
	return ev.ExitCode != -1
}

func (ev EventProcessExited) Type() string { return eventProcessExited }
func (ev EventProcessExited) event()       {}

// EventProcessListModify is emitted when the process list is modified to add,
// update or remove a process from the internal state.
type EventProcessListModify struct {
	Op   ProcessListModifyOp `json:"op"`
	File string              `json:"file"`
}

// ProcessListModifyOp contains possible operations that modify the process
// list, often from changes in the configuration directory.
type ProcessListModifyOp string

const (
	ProcessListAdd    ProcessListModifyOp = "add"
	ProcessListRemove ProcessListModifyOp = "remove"
	ProcessListUpdate ProcessListModifyOp = "update"
)

func (ev EventProcessListModify) Type() string { return eventProcessListModify }
func (ev EventProcessListModify) event()       {}
