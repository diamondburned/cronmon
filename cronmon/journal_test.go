package cronmon

import (
	"io"
	"reflect"
	"sync"
	"testing"
	"time"
)

// mockJournal is an in-memory storage of journals, primarily used for testing.
// A zero-value instance is a valid instance.
type mockJournal struct {
	mutex    sync.Mutex
	finalize bool
	journals []Event
}

var _ Journaler = (*mockJournal)(nil)

func (m *mockJournal) ID() string { return "mock" }

// Finalize locks the memory store. Future writes will cause a panic.
func (m *mockJournal) Finalize() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.finalize = true
}

// Write appends a journal event into the internal store.
func (m *mockJournal) Write(ev Event) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.finalize {
		panic("log write when finalized")
	}

	m.journals = append(m.journals, ev)
	return nil
}

// Journals returns the journal slice.
func (m *mockJournal) Journals() []Event {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	return m.journals
}

// Verify verifies that the given journals slice is equal to the one stored
// internally. If strict is true, then a length check is performed, otherwise,
// the unmatched events are returned.
//
// Consecutive calls to Verify will match the remaining unmatched events.
func (m *mockJournal) Verify(t *testing.T, strict bool, journals []Event) []Event {
	t.Helper()

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if strict && len(journals) != len(m.journals) {
		t.Errorf("mismatch journal length, got %d, expected %d", len(m.journals), len(journals))
		return nil
	}

	for i, ev := range journals {
		if !reflect.DeepEqual(m.journals[i], ev) {
			t.Errorf("journal %d mismatch, got %#v, expected %#v", i, m.journals[i], ev)
		}
	}

	m.journals = m.journals[len(journals):]
	return m.journals
}

func TestReadPreviousState(t *testing.T) {
	events := []Event{
		&EventProcessSpawned{PID: 2, File: "a"},
		&EventProcessExited{PID: 2, File: "a"},
		&EventProcessExited{PID: 3, File: "b"},
		&EventProcessSpawned{PID: 2, File: "a"},
		&EventProcessSpawned{PID: 3, File: "b"},
		&EventAcquired{},
	}

	d := time.Date(2020, 04, 01, 00, 00, 00, 00, time.UTC)
	r := mockReader{
		events: make([]mockEvent, len(events)),
	}
	for i, ev := range events {
		r.events[i] = mockEvent{e: ev, t: d}
	}

	state, err := ReadPreviousState(&r)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	expect := &PreviousState{
		StartedAt: d,
		Processes: map[string]int{"a": 2},
	}

	if !reflect.DeepEqual(state, expect) {
		t.Fatalf("unexpected state returned:\n"+
			"got      %#v\n"+
			"expected %#v", state, expect)
	}
}

type mockReader struct {
	events []mockEvent
	cursor int
}

type mockEvent struct {
	e Event
	t time.Time
}

func (r *mockReader) Read() (Event, time.Time, error) {
	if r.cursor >= len(r.events) {
		return nil, time.Time{}, io.EOF
	}

	ev := r.events[r.cursor]
	r.cursor++

	return ev.e, ev.t, nil
}
