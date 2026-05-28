package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/conf"
)

func TestConfigLoadsGatewayYAML(t *testing.T) {
	t.Parallel()

	var c Config
	require.NoError(t, conf.Load("../../etc/gateway-api.yaml", &c))
	require.Equal(t, "gateway-api", c.Name)
	require.Equal(t, "0.0.0.0", c.Host)
	require.Equal(t, 8888, c.Port)
	require.Equal(t, "gateway-api", c.Telemetry.Name)
	require.Equal(t, "tempo:4318", c.Telemetry.Endpoint)
	require.InEpsilon(t, 1.0, c.Telemetry.Sampler, 0.0001)
	require.Equal(t, "otlphttp", c.Telemetry.Batcher)
	require.Equal(t, "/v1/traces", c.Telemetry.OtlpHttpPath)
	require.Equal(t, "nacos:8848", c.AuthRpc.ServerAddr)
	require.Empty(t, c.AuthRpc.AdvertiseIP)
	require.Equal(t, "aim-dev-access-secret", c.Auth.AccessSecret)
	require.Equal(t, "nacos:8848", c.AttachmentRpc.ServerAddr)
	require.Equal(t, "attachment.rpc", c.AttachmentRpc.ServiceName)
	require.Empty(t, c.WebSocket.OriginPatterns)
	require.Equal(t, int64(32768), c.WebSocket.ReadLimit)
	require.Equal(t, int64(32768), c.WebSocket.WriteLimit)
	require.Equal(t, 30, c.WebSocket.PongWait)
	require.Equal(t, 15, c.WebSocket.PingPeriod)
	require.Equal(t, int64(1024), c.WebSocket.MaxMsgSize)
	require.Equal(t, 5, c.WebSocket.ServerAckDelay)
	require.Equal(t, 30, c.WebSocket.HeartbeatInterv)
	require.Equal(t, "gateway.rpc", c.GatewayRpc.Name)
	require.Equal(t, "0.0.0.0:9091", c.GatewayRpc.ListenOn)
	require.Equal(t, "gateway.rpc", c.GatewayRpc.Telemetry.Name)
	require.Equal(t, "tempo:4318", c.GatewayRpc.Telemetry.Endpoint)
	require.InEpsilon(t, 1.0, c.GatewayRpc.Telemetry.Sampler, 0.0001)
	require.Equal(t, "otlphttp", c.GatewayRpc.Telemetry.Batcher)
	require.Equal(t, "/v1/traces", c.GatewayRpc.Telemetry.OtlpHttpPath)
	// Prometheus top-level
	require.Equal(t, "0.0.0.0", c.Prometheus.Host)
	require.Equal(t, 9191, c.Prometheus.Port)
	require.Equal(t, "/metrics", c.Prometheus.Path)
	// Prometheus GatewayRpc
	require.Equal(t, "0.0.0.0", c.GatewayRpc.Prometheus.Host)
	require.Equal(t, 9191, c.GatewayRpc.Prometheus.Port)
	require.Equal(t, "/metrics", c.GatewayRpc.Prometheus.Path)
}
