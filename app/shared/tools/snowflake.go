package tools

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	// SnowflakeDefaultEpoch is 2025-01-01 00:00:00 UTC in milliseconds.
	SnowflakeDefaultEpoch int64 = 1735689600000

	snowflakeMachineIDBits = 10
	snowflakeSequenceBits  = 12
	snowflakeMaxMachineID  = int64(1<<snowflakeMachineIDBits - 1)
	snowflakeMaxSequence   = int64(1<<snowflakeSequenceBits - 1)
	snowflakeMachineShift  = snowflakeSequenceBits
	snowflakeTimeShift     = snowflakeMachineIDBits + snowflakeSequenceBits

	// Default clock rollback tolerance: 5ms. Small NTP corrections are common.
	defaultClockBackwardToleranceMs int64 = 5
	// Default wait timeout for large clock rollback: 10s.
	defaultClockBackwardWaitTimeout = 10 * time.Second
)

var (
	// ErrInvalidSnowflakeMachineID indicates the machine ID is outside the 10-bit Snowflake range.
	ErrInvalidSnowflakeMachineID = errors.New("invalid snowflake machine id")
	// ErrSnowflakeClockMovedBackwards indicates the system clock moved backwards beyond the tolerance threshold.
	ErrSnowflakeClockMovedBackwards = errors.New("snowflake clock moved backwards")
)

// Snowflake generates unique 64-bit IDs.
//
// Layout: 1 sign bit, 41 timestamp bits, 10 machine ID bits, and 12 sequence bits.
//
// Clock Rollback Handling:
//   - Small rollback (≤ toleranceMs): uses lastTime as logical clock + increments sequence.
//     IDs remain monotonically increasing; embedded timestamp reflects lastTime, not real time.
//   - Large rollback (> toleranceMs): waits for the system clock to catch up to lastTime.
//     If the wait exceeds waitTimeout, returns ErrSnowflakeClockMovedBackwards.
type Snowflake struct {
	mu        sync.Mutex
	machineID int64
	epoch     int64
	now       func() int64
	lastTime  int64
	sequence  int64

	// clockBackwardToleranceMs is the maximum clock drift (ms) tolerated via logical clock.
	clockBackwardToleranceMs int64
	// clockBackwardWaitTimeout is the max duration to wait for clock recovery.
	clockBackwardWaitTimeout time.Duration
}

// SnowflakeOption configures a Snowflake generator.
type SnowflakeOption func(*Snowflake)

// WithClockBackwardTolerance sets the maximum clock rollback (in milliseconds)
// that will be tolerated via logical clock instead of returning an error.
// A value of 0 disables tolerance (strict mode, original behavior).
func WithClockBackwardTolerance(ms int64) SnowflakeOption {
	return func(s *Snowflake) {
		s.clockBackwardToleranceMs = ms
	}
}

// WithClockBackwardWaitTimeout sets the maximum duration to wait when the
// system clock moves backwards beyond the tolerance threshold.
// A value of 0 means fail immediately without waiting.
func WithClockBackwardWaitTimeout(d time.Duration) SnowflakeOption {
	return func(s *Snowflake) {
		s.clockBackwardWaitTimeout = d
	}
}

// NewSnowflake creates a new Snowflake generator with the given machine ID.
// Default: tolerates up to 5ms clock rollback, waits up to 10s for recovery.
func NewSnowflake(machineID int64, opts ...SnowflakeOption) (*Snowflake, error) {
	return NewSnowflakeWithEpoch(machineID, SnowflakeDefaultEpoch, opts...)
}

// NewSnowflakeWithEpoch creates a new Snowflake generator with a custom epoch in milliseconds.
func NewSnowflakeWithEpoch(machineID, epoch int64, opts ...SnowflakeOption) (*Snowflake, error) {
	if machineID < 0 || machineID > snowflakeMaxMachineID {
		return nil, ErrInvalidSnowflakeMachineID
	}

	s := &Snowflake{
		machineID:                machineID,
		epoch:                    epoch,
		now:                      currentMilliseconds,
		clockBackwardToleranceMs: defaultClockBackwardToleranceMs,
		clockBackwardWaitTimeout: defaultClockBackwardWaitTimeout,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// NextID generates the next unique Snowflake ID.
func (s *Snowflake) NextID() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()

	if now < s.lastTime {
		backwardMs := s.lastTime - now
		if backwardMs <= s.clockBackwardToleranceMs {
			// Small rollback: use logical clock (lastTime) + increment sequence.
			// ID remains monotonically increasing.
			now = s.lastTime
		} else {
			// Large rollback: wait for the clock to catch up.
			if err := s.waitForClockRecovery(); err != nil {
				return 0, err
			}
			now = s.now()
		}
	}

	if now == s.lastTime {
		s.sequence = (s.sequence + 1) & snowflakeMaxSequence
		if s.sequence == 0 {
			now = s.waitNextMillisecond(now)
		}
	} else {
		s.sequence = 0
	}

	s.lastTime = now

	return ((now - s.epoch) << snowflakeTimeShift) |
		(s.machineID << snowflakeMachineShift) |
		s.sequence, nil
}

// waitForClockRecovery blocks until the system clock catches up to lastTime,
// or until the wait timeout is exceeded.
func (s *Snowflake) waitForClockRecovery() error {
	if s.clockBackwardWaitTimeout <= 0 {
		return ErrSnowflakeClockMovedBackwards
	}

	deadline := time.Now().Add(s.clockBackwardWaitTimeout)
	for time.Now().Before(deadline) {
		if s.now() >= s.lastTime {
			return nil
		}
		time.Sleep(time.Millisecond)
	}
	return fmt.Errorf("%w (waited %v, clock still %dms behind)",
		ErrSnowflakeClockMovedBackwards, s.clockBackwardWaitTimeout, s.lastTime-s.now())
}

func (s *Snowflake) waitNextMillisecond(now int64) int64 {
	for now <= s.lastTime {
		now = s.now()
	}

	return now
}

func currentMilliseconds() int64 {
	return time.Now().UnixMilli()
}
