package exec

import (
	"os"
)

// Process describes a command process.
type Process interface {
	PID() int
	Signal(os.Signal) error
	Kill() error
	Wait() ExitStatus
}

// ExitStatus is a process' exit status.
type ExitStatus struct {
	PID   int
	Code  int // -1 for interrupt
	Error error
}

type process struct {
	*os.Process
}

var _ Process = process{}

// FindProcess creates a new Process from an existing process ID.
func FindProcess(pid int) (Process, error) {
	p, err := os.FindProcess(pid)
	if err != nil {
		return nil, err
	}

	return process{p}, nil
}

// StartProcess creates a new command process on the system.
func StartProcess(argv []string) (Process, error) {
	p, err := os.StartProcess(argv[0], argv, &os.ProcAttr{})
	if err != nil {
		return nil, err
	}

	return process{p}, nil
}

func (proc process) PID() int {
	return proc.Pid
}

func (proc process) Wait() ExitStatus {
	status := ExitStatus{
		PID:  proc.Pid,
		Code: -1,
	}

	s, err := proc.Process.Wait()
	if err != nil {
		status.Error = err
	} else {
		status.Code = s.ExitCode()
	}

	return status
}
