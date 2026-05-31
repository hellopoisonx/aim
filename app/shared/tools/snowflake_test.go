package tools

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSnowflakeGeneratesIncreasingIDs(t *testing.T) {
	generator, err := NewSnowflake(1)
	require.NoError(t, err)

	first, err := generator.NextID()
	require.NoError(t, err)

	second, err := generator.NextID()
	require.NoError(t, err)

	require.Positive(t, first)
	require.Greater(t, second, first)
}

func TestSnowflakeGeneratesUniqueIDsConcurrently(t *testing.T) {
	generator, err := NewSnowflake(2)
	require.NoError(t, err)

	const workers = 16

	const idsPerWorker = 256

	ids := make(chan int64, workers*idsPerWorker)

	var wg sync.WaitGroup

	for range workers {
		wg.Go(func() {
			for range idsPerWorker {
				id, err := generator.NextID()
				require.NoError(t, err)

				ids <- id
			}
		})
	}

	wg.Wait()
	close(ids)

	seen := make(map[int64]struct{}, workers*idsPerWorker)
	for id := range ids {
		_, ok := seen[id]
		require.False(t, ok, "duplicate id: %d", id)
		seen[id] = struct{}{}
	}

	require.Len(t, seen, workers*idsPerWorker)
}

func TestSnowflakeRejectsInvalidMachineID(t *testing.T) {
	_, err := NewSnowflake(-1)
	require.ErrorIs(t, err, ErrInvalidSnowflakeMachineID)

	_, err = NewSnowflake(snowflakeMaxMachineID + 1)
	require.ErrorIs(t, err, ErrInvalidSnowflakeMachineID)
}

func TestSnowflakeToleratesSmallClockRollback(t *testing.T) {
	// 1ms rollback is within default 5ms tolerance → should NOT error.
	generator, err := NewSnowflake(3)
	require.NoError(t, err)

	times := []int64{SnowflakeDefaultEpoch + 10, SnowflakeDefaultEpoch + 9}
	callCount := 0
	generator.now = func() int64 {
		next := times[callCount]
		callCount++
		return next
	}

	first, err := generator.NextID()
	require.NoError(t, err)

	second, err := generator.NextID()
	require.NoError(t, err)

	// IDs must be monotonically increasing even when clock rolls back.
	require.Greater(t, second, first)
}

func TestSnowflakeWaitsOnLargeClockRollback(t *testing.T) {
	// 100ms rollback exceeds 5ms tolerance → should wait then timeout.
	generator, err := NewSnowflake(4,
		WithClockBackwardTolerance(5),
		WithClockBackwardWaitTimeout(0), // fail immediately for test speed
	)
	require.NoError(t, err)

	times := []int64{SnowflakeDefaultEpoch + 100, SnowflakeDefaultEpoch}
	callCount := 0
	generator.now = func() int64 {
		next := times[callCount]
		callCount++
		return next
	}

	_, err = generator.NextID()
	require.NoError(t, err)

	_, err = generator.NextID()
	require.ErrorIs(t, err, ErrSnowflakeClockMovedBackwards)
}

func TestSnowflakeRecoversAfterClockRollbackWait(t *testing.T) {
	// 10ms rollback exceeds tolerance → waits → clock recovers → should succeed.
	generator, err := NewSnowflake(5,
		WithClockBackwardTolerance(5),
		WithClockBackwardWaitTimeout(100*time.Millisecond),
	)
	require.NoError(t, err)

	// Phase-based mock: first call returns t+100, then t+90 (rollback), then t+101 (recovery).
	// waitForClockRecovery loops calling now(), so we need a persistent value during the rollback phase.
	callCount := 0
	generator.now = func() int64 {
		callCount++
		switch callCount {
		case 1:
			return SnowflakeDefaultEpoch + 100
		case 2:
			return SnowflakeDefaultEpoch + 90
		default:
			return SnowflakeDefaultEpoch + 101
		}
	}

	// First ID at t+100.
	first, err := generator.NextID()
	require.NoError(t, err)

	// Clock rolled back to t+90 (10ms > 5ms tolerance), waitForClockRecovery polls again → t+101.
	second, err := generator.NextID()
	require.NoError(t, err)

	require.Greater(t, second, first)
}

func TestSnowflakeStrictMode(t *testing.T) {
	// Tolerance=0 → any rollback fails immediately.
	generator, err := NewSnowflake(6,
		WithClockBackwardTolerance(0),
		WithClockBackwardWaitTimeout(0),
	)
	require.NoError(t, err)

	times := []int64{SnowflakeDefaultEpoch + 10, SnowflakeDefaultEpoch + 9}
	callCount := 0
	generator.now = func() int64 {
		next := times[callCount]
		callCount++
		return next
	}

	_, err = generator.NextID()
	require.NoError(t, err)

	_, err = generator.NextID()
	require.ErrorIs(t, err, ErrSnowflakeClockMovedBackwards)
}
