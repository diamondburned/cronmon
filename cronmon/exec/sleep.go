package exec

import (
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type sleepProcess struct {
	once  sync.Once
	stop  chan struct{}
	timer *time.Timer
	delay time.Duration

	pid  int
	exit int32
}

// NewSleepProcess creates a process that only idles for a duration. It is used
// for testing. If delay is larger than 0, then the process will sleep for that
// delay before exiting, unless it is SIGKILLed.
func NewSleepProcess(dura, delay time.Duration, pid int) Process {
	return &sleepProcess{
		stop:  make(chan struct{}),
		timer: time.NewTimer(dura),
		delay: delay,

		pid:  pid,
		exit: -2,
	}
}

func (mock *sleepProcess) PID() int { return mock.pid }

func (mock *sleepProcess) Signal(sig os.Signal) error {
	var status int32

	switch sig {
	case os.Interrupt:
		status = 0
	case os.Kill:
		status = -1
	default:
		return errors.New("unknown signal")
	}

	go func() {
		if mock.delay > 0 && sig != os.Kill {
			select {
			case <-time.After(mock.delay):

			case <-mock.stop:
				return
			}
		}

		// Ensure exit is still unset (-2), otherwise bail.
		if !atomic.CompareAndSwapInt32(&mock.exit, -2, status) {
			return
		}

		close(mock.stop)
		mock.timer.Stop()
	}()

	return nil
}

func (mock *sleepProcess) Kill() error {
	return mock.Signal(os.Kill)
}

func (mock *sleepProcess) Wait() ExitStatus {
	mock.once.Do(func() {
		select {
		case <-mock.stop:
		case <-mock.timer.C:
			atomic.StoreInt32(&mock.exit, 0)
		}
	})

	return ExitStatus{
		PID:  mock.pid,
		Code: int(atomic.LoadInt32(&mock.exit)),
	}
}
