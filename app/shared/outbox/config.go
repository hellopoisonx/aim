package outbox

import "time"

const (
	defaultBatchSize       = 100
	defaultPollInterval    = 200 * time.Millisecond
	defaultCleanupInterval = 10 * time.Minute
	defaultCleanupMaxAge   = 24 * time.Hour
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
	return c
}
