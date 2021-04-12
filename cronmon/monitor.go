package cronmon

import "sync"

// Monitor is a cronmon instance that monitors a set of processes.
type Monitor struct {
	procs sync.Map // file -> *Process
}
