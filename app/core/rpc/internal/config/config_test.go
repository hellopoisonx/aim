package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/conf"
)

func TestConfigLoadsCoreYAML(t *testing.T) {
	t.Parallel()

	var c Config
	require.NoError(t, conf.Load("../../etc/core.yaml", &c))
	require.Equal(t, "core.rpc", c.Name)
	require.Equal(t, "0.0.0.0:8080", c.ListenOn)
	require.Equal(t, "nacos:8848", c.Nacos.ServerAddr)
	require.Equal(t, "nacos:8848", c.LogicRpc.ServerAddr)
	require.Equal(t, "logic.rpc", c.LogicRpc.ServiceName)
	require.Equal(t, "redis:6379", c.CacheRedis.Addr)
	require.Equal(t, []string{"kafka:9092"}, c.KqPusherConf.Brokers)
	require.Equal(t, "aim-message-transfer", c.KqPusherConf.Topic)
	require.Equal(t, "gateway.rpc", c.GatewayRpc.ServiceName)
	require.Equal(t, "aim-gateway:9090", c.GatewayRpc.Target)
	require.Equal(t, "core.rpc", c.Telemetry.Name)
	require.Equal(t, "jaeger:4318", c.Telemetry.Endpoint)
	require.InEpsilon(t, 1.0, c.Telemetry.Sampler, 0.0001)
	require.Equal(t, "otlphttp", c.Telemetry.Batcher)
	require.Equal(t, "/v1/traces", c.Telemetry.OtlpHttpPath)
}
