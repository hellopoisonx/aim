package cache

const (
	// DefaultInvalidateStream is the Redis Stream used to broadcast cache invalidations.
	DefaultInvalidateStream = "aim:cache:invalidate"

	defaultL1Capacity   = 1000
	defaultL1TTLSeconds = 30
	defaultL2TTLSeconds = 300
)

// Config controls the shared two-level cache.
type Config struct {
	L1Capacity       int
	L1TTLSeconds     int
	L2TTLSeconds     int
	InvalidateStream string
}

// WithDefaults fills zero values with production-safe defaults.
func (c Config) WithDefaults() Config {
	if c.L1Capacity <= 0 {
		c.L1Capacity = defaultL1Capacity
	}
	if c.L1TTLSeconds <= 0 {
		c.L1TTLSeconds = defaultL1TTLSeconds
	}
	if c.L2TTLSeconds <= 0 {
		c.L2TTLSeconds = defaultL2TTLSeconds
	}
	if c.InvalidateStream == "" {
		c.InvalidateStream = DefaultInvalidateStream
	}

	return c
}
