package main

import (
	"testing"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestCorsOptionsDisabled(t *testing.T) {
	t.Parallel()

	opts := corsOptions(config.CorsConf{Enabled: false})
	assert.Nil(t, opts, "Disabled CORS must return no options")
}

func TestCorsOptionsEnabledDefaultOrigins(t *testing.T) {
	t.Parallel()

	opts := corsOptions(config.CorsConf{Enabled: true})
	assert.Len(t, opts, 1, "Enabled with empty origins returns one option (wildcard)")
}

func TestCorsOptionsEnabledSpecificOrigins(t *testing.T) {
	t.Parallel()

	opts := corsOptions(config.CorsConf{
		Enabled: true,
		Origins: []string{"https://example.com", "https://app.example.com"},
	})
	assert.Len(t, opts, 1, "Enabled with explicit origins returns one option")
}
