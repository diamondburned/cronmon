package cronmon

import (
	"encoding/json"
	"fmt"
	"time"
)

// eventType describes an event type.
type eventType = string

const (
	eventAcquired          eventType = "acquired lock"
	eventLogTruncated      eventType = "log truncated"
	eventProcessSpawned    eventType = "process spawned"
	eventProcessStopped    eventType = "process stopped"
	eventProcessListModify eventType = "process list modified"
)

// Event is an interface describing known events.
type Event interface {
	Type() string
	event()
}

// EventJSON describes the type and data of a cronmon event in the JSON format.
type EventJSON struct {
	Time time.Time `json:"time"`
	Type string    `json:"type"`
	Data Event     `json:"data"`
}

var eventFuncs = map[eventType]func() Event{
	eventAcquired:          func() Event { return &EventAcquired{} },
	eventLogTruncated:      func() Event { return &EventLogTruncated{} },
	eventProcessSpawned:    func() Event { return &EventProcessSpawned{} },
	eventProcessStopped:    func() Event { return &EventProcessStopped{} },
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

// EventProcessSpawned is emitted when a process has been started for any
// reason.
type EventProcessSpawned struct {
	PID  int    `json:"pid"`
	File string `json:"file"`
}

func (ev EventProcessSpawned) Type() string { return eventProcessSpawned }
func (ev EventProcessSpawned) event()       {}

// EventProcessStopped is emitted when a process has been stopped for any
// reason.
type EventProcessStopped struct {
	PID  int    `json:"pid"`
	File string `json:"file"`
}

func (ev EventProcessStopped) Type() string { return eventProcessStopped }
func (ev EventProcessStopped) event()       {}

// EventProcessListModify is emitted when the process list is modified to add,
// update or remove a process from the internal state.
type EventProcessListModify struct {
	Op   string `json:"op"`
	File string `json:"file"`
}

func (ev EventProcessListModify) Type() string { return eventProcessListModify }
func (ev EventProcessListModify) event()       {}
