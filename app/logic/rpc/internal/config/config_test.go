package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/conf"
)

func TestConfigLoadsLogicYAML(t *testing.T) {
	t.Parallel()

	var c Config
	require.NoError(t, conf.Load("../../etc/logic.yaml", &c))
	require.Equal(t, "logic.rpc", c.Name)
	require.Equal(t, "0.0.0.0:8082", c.ListenOn)
	require.Equal(t, "logic.rpc", c.Etcd.Key)
	require.Equal(t, []string{"etcd:2379"}, c.Etcd.Hosts)
	require.Equal(t, "postgres://user:password@postgres:5432/aim_logic?sslmode=disable", c.Postgres.DataSource)
	require.Equal(t, "redis:6379", c.CacheRedis.Addr)
	require.Equal(t, "logic.rpc", c.Telemetry.Name)
	require.Equal(t, "tempo:4318", c.Telemetry.Endpoint)
	require.InEpsilon(t, 1.0, c.Telemetry.Sampler, 0.0001)
	require.Equal(t, "otlphttp", c.Telemetry.Batcher)
	require.Equal(t, "/v1/traces", c.Telemetry.OtlpHttpPath)
	require.Equal(t, "0.0.0.0", c.Prometheus.Host)
	require.Equal(t, 9194, c.Prometheus.Port)
	require.Equal(t, "/metrics", c.Prometheus.Path)
}

func TestConfigUserCreatedConsumerConf(t *testing.T) {
	t.Parallel()

	var c Config
	require.NoError(t, conf.Load("../../etc/logic.yaml", &c))
	require.True(t, c.IsUserCreatedConsumerConfigured())
	require.Equal(t, "logic-user-created-consumer", c.UserCreatedConsumerConf.Name)
	require.Equal(t, []string{"kafka:9092"}, c.UserCreatedConsumerConf.Brokers)
	require.Equal(t, "aim-logic-user-created", c.UserCreatedConsumerConf.Group)
	require.Equal(t, "aim.user.events", c.UserCreatedConsumerConf.Topic)
	require.Equal(t, "first", c.UserCreatedConsumerConf.Offset)
	require.Equal(t, 1, c.UserCreatedConsumerConf.Consumers)
	require.Equal(t, 1, c.UserCreatedConsumerConf.Processors)
}

func TestConfigUserCreatedConsumerConf_NotConfigured(t *testing.T) {
	t.Parallel()

	c := Config{}
	require.False(t, c.IsUserCreatedConsumerConfigured())
}
