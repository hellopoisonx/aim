package tools

import (
	"errors"
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
)

var (
	// ErrInvalidSnowflakeMachineID indicates the machine ID is outside the 10-bit Snowflake range.
	ErrInvalidSnowflakeMachineID = errors.New("invalid snowflake machine id")
	// ErrSnowflakeClockMovedBackwards indicates the system clock moved before the last generated ID time.
	ErrSnowflakeClockMovedBackwards = errors.New("snowflake clock moved backwards")
)

// Snowflake generates unique 64-bit IDs.
//
// Layout: 1 sign bit, 41 timestamp bits, 10 machine ID bits, and 12 sequence bits.
type Snowflake struct {
	mu        sync.Mutex
	machineID int64
	epoch     int64
	now       func() int64
	lastTime  int64
	sequence  int64
}

// NewSnowflake creates a new Snowflake generator with the given machine ID.
func NewSnowflake(machineID int64) (*Snowflake, error) {
	return NewSnowflakeWithEpoch(machineID, SnowflakeDefaultEpoch)
}

// NewSnowflakeWithEpoch creates a new Snowflake generator with a custom epoch in milliseconds.
func NewSnowflakeWithEpoch(machineID, epoch int64) (*Snowflake, error) {
	if machineID < 0 || machineID > snowflakeMaxMachineID {
		return nil, ErrInvalidSnowflakeMachineID
	}

	return &Snowflake{
		machineID: machineID,
		epoch:     epoch,
		now:       currentMilliseconds,
	}, nil
}

// NextID generates the next unique Snowflake ID.
func (s *Snowflake) NextID() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	if now < s.lastTime {
		return 0, ErrSnowflakeClockMovedBackwards
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

func (s *Snowflake) waitNextMillisecond(now int64) int64 {
	for now <= s.lastTime {
		now = s.now()
	}

	return now
}

func currentMilliseconds() int64 {
	return time.Now().UnixMilli()
}
