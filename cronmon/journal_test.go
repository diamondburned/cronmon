package cronmon

import (
	"reflect"
	"sync"
	"testing"
)

// mockJournal is an in-memory storage of journals, primarily used for testing.
// A zero-value instance is a valid instance.
type mockJournal struct {
	mutex    sync.Mutex
	finalize bool
	journals []Event
}

var _ Journaler = (*mockJournal)(nil)

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
