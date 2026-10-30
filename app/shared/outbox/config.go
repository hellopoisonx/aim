package outbox

import "time"

const (
	defaultBatchSize        = 100
	defaultPollInterval     = 200 * time.Millisecond
	defaultCleanupInterval  = 10 * time.Minute
	defaultCleanupMaxAge    = 24 * time.Hour
	defaultRetryBackoffBase = 500 * time.Millisecond
	defaultRetryBackoffMax  = 30 * time.Second
)

// Config controls the outbox Poller.
type Config struct {
	// BatchSize is the maximum number of records fetched per poll cycle.
	BatchSize int

	// PollInterval is the wait between poll cycles.
	PollInterval time.Duration

	// CleanupInterval is how often processed records are pruned.
	// Set to a negative value to disable cleanup; zero uses the default (10m).
	CleanupInterval time.Duration

	// CleanupMaxAge is the minimum age of processed records before deletion.
	CleanupMaxAge time.Duration

	// RetryBackoffBase is the delay applied after the first publish failure of
	// a record. Each further failure doubles it up to RetryBackoffMax.
	RetryBackoffBase time.Duration

	// RetryBackoffMax caps the per-record retry delay.
	RetryBackoffMax time.Duration
}

// WithDefaults fills zero values with production-safe defaults.
func (c Config) WithDefaults() Config {
	if c.BatchSize <= 0 {
		c.BatchSize = defaultBatchSize
	}
	if c.PollInterval <= 0 {
		c.PollInterval = defaultPollInterval
	}
	cleanupDisabled := c.CleanupInterval < 0
	if cleanupDisabled {
		c.CleanupInterval = 0 // negative means disabled
	} else {
		if c.CleanupInterval == 0 {
			c.CleanupInterval = defaultCleanupInterval
		}
		if c.CleanupMaxAge <= 0 {
			c.CleanupMaxAge = defaultCleanupMaxAge
		}
	}
	if c.RetryBackoffBase <= 0 {
		c.RetryBackoffBase = defaultRetryBackoffBase
	}
	if c.RetryBackoffMax <= 0 {
		c.RetryBackoffMax = defaultRetryBackoffMax
	}
	if c.RetryBackoffMax < c.RetryBackoffBase {
		c.RetryBackoffMax = c.RetryBackoffBase
	}
	return c
}

// retryDelay returns the backoff delay after attempts consecutive failures.
func (c Config) retryDelay(attempts int) time.Duration {
	delay := c.RetryBackoffBase
	for i := 1; i < attempts && delay < c.RetryBackoffMax; i++ {
		delay *= 2
	}
	if delay > c.RetryBackoffMax {
		return c.RetryBackoffMax
	}
	return delay
}
