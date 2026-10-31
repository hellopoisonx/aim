package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/conf"
)

func TestConfigLoadsAuthYAML(t *testing.T) {
	t.Parallel()

	var c Config
	require.NoError(t, conf.Load("../../etc/auth.yaml", &c))
	require.Equal(t, "auth.rpc", c.Name)
	require.Equal(t, "0.0.0.0:8989", c.ListenOn)
	require.Equal(t, "redis:6379", c.SessionRedis.Host)
	require.Equal(t, "aim-dev-access-secret", c.Token.AccessSecret)
	require.Equal(t, "auth.rpc", c.Etcd.Key)
	require.Equal(t, []string{"etcd:2379"}, c.Etcd.Hosts)
	require.Equal(t, "auth.rpc", c.Telemetry.Name)
	require.Equal(t, "tempo:4318", c.Telemetry.Endpoint)
	require.InEpsilon(t, 1.0, c.Telemetry.Sampler, 0.0001)
	require.Equal(t, "otlphttp", c.Telemetry.Batcher)
	require.Equal(t, "/v1/traces", c.Telemetry.OtlpHttpPath)
	//nolint:staticcheck
	require.Equal(t, "0.0.0.0", c.Prometheus.Host)
	//nolint:staticcheck
	require.Equal(t, 9192, c.Prometheus.Port)
	//nolint:staticcheck
	require.Equal(t, "/metrics", c.Prometheus.Path)
}

func TestConfigUserEventProducerConf(t *testing.T) {
	t.Parallel()

	var c Config
	require.NoError(t, conf.Load("../../etc/auth.yaml", &c))
	require.True(t, c.IsUserEventProducerConfigured())
	require.Equal(t, "rocketmq-proxy:8081", c.UserEventProducerConf.Endpoint)
	require.Equal(t, "aim-auth-producer", c.UserEventProducerConf.Group)
	require.Equal(t, "aim-user-events", c.UserEventProducerConf.Topic)
}

func TestConfigUserEventProducerConf_NotConfigured(t *testing.T) {
	t.Parallel()

	c := Config{}
	require.False(t, c.IsUserEventProducerConfigured())
	require.Empty(t, c.UserEventProducerConf.Endpoint)
	require.Empty(t, c.UserEventProducerConf.Topic)
}
