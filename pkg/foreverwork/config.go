package foreverwork

import "time"

// Config describes one worker pool.
type Config struct {
	// Name appears on every line the pool logs and in Result, so several pools
	// are told apart in one stream.
	Name string
	// Workers is the number of goroutines, which is the number of jobs running
	// at the same time. The count is the limit, so nothing counts running jobs.
	Workers int
	// QueueSize is the buffered channel's capacity: how many jobs may wait.
	// Small on purpose — Submit blocks when it is full, and that backpressure
	// is what stops a producer from running ahead of what the pool can do.
	QueueSize int
	// Timeout is the deadline for one job, and every pool has one: a handler
	// waiting on a hung dependency otherwise holds its runner for good, and a
	// pool that has lost all its runners that way looks idle while doing
	// nothing. Zero means DefaultTimeout rather than "no deadline", because
	// "no deadline" is never what a long-lived pool wants.
	Timeout time.Duration
}

// DefaultTimeout is the ceiling a pool gets when it names none.
const DefaultTimeout = 5 * time.Minute

// DefaultShutdownGrace is the shutdown budget a pool gets when it names none. It
// sits under Kubernetes' default terminationGracePeriodSeconds of 30, so a pool
// gives up its jobs before the pod is killed rather than after.
const DefaultShutdownGrace = 20 * time.Second

func (c Config) withDefaults() Config {
	if c.Name == "" {
		c.Name = "worker"
	}
	if c.Workers < 1 {
		c.Workers = 1
	}
	// At least one slot: an unbuffered queue would make every submit wait for a
	// free worker, which turns a hand-off into a rendezvous.
	if c.QueueSize < 1 {
		c.QueueSize = 1
	}
	if c.Timeout <= 0 {
		c.Timeout = DefaultTimeout
	}
	return c
}
