package cronmon

import (
	"reflect"
	"sync"
	"testing"
)

type mockJournaler struct {
	mutex    sync.Mutex
	finalize bool
	journals []Event
}

var _ Journaler = (*mockJournaler)(nil)

func (j *mockJournaler) Write(ev Event) error {
	j.mutex.Lock()
	defer j.mutex.Unlock()

	if j.finalize {
		panic("log write when finalized")
	}

	j.journals = append(j.journals, ev)
	return nil
}

func (j *mockJournaler) Finalize() []Event {
	j.mutex.Lock()
	defer j.mutex.Unlock()

	j.finalize = true

	return j.journals
}

func (j *mockJournaler) Verify(t *testing.T, strict bool, journals []Event) []Event {
	t.Helper()

	j.mutex.Lock()
	defer j.mutex.Unlock()

	j.finalize = true

	if strict && len(journals) != len(j.journals) {
		t.Errorf("mismatch journal length, got %d, expected %d", len(j.journals), len(journals))
		return nil
	}

	for i, ev := range journals {
		if !reflect.DeepEqual(j.journals[i], ev) {
			t.Errorf("journal %d mismatch, got %#v, expected %#v", i, j.journals[i], ev)
		}
	}

	j.journals = j.journals[len(journals):]
	return j.journals
}
