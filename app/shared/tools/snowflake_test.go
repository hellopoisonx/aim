package tools

import (
	"sync"
	"testing"

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

func TestSnowflakeRejectsClockRollback(t *testing.T) {
	generator, err := NewSnowflake(3)
	require.NoError(t, err)

	times := []int64{SnowflakeDefaultEpoch + 10, SnowflakeDefaultEpoch + 9}
	generator.now = func() int64 {
		next := times[0]
		times = times[1:]

		return next
	}

	_, err = generator.NextID()
	require.NoError(t, err)

	_, err = generator.NextID()
	require.ErrorIs(t, err, ErrSnowflakeClockMovedBackwards)
}
