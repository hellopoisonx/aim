package nacos

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfig_ApplyDefaults(t *testing.T) {
	t.Parallel()

	var c Config
	require.NoError(t, c.ApplyDefaults("auth.rpc", "0.0.0.0:8080"))

	require.Equal(t, "127.0.0.1:8848", c.ServerAddr)
	require.Equal(t, defaultGroup, c.Group)
	require.Equal(t, defaultCluster, c.Cluster)
	require.Equal(t, "auth.rpc", c.ServiceName)
	require.Equal(t, "127.0.0.1", c.AdvertiseIP)
	require.Equal(t, uint64(8080), c.AdvertisePort)
	require.InEpsilon(t, 1, c.Weight, 0)
	require.Equal(t, uint64(5*time.Second/time.Millisecond), c.TimeoutMs)
}

func TestSplitListenOn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		listenOn string
		wantIP   string
		wantPort uint64
	}{
		{name: "wildcard ipv4", listenOn: "0.0.0.0:8080", wantIP: "127.0.0.1", wantPort: 8080},
		{name: "explicit host", listenOn: "10.0.0.2:8989", wantIP: "10.0.0.2", wantPort: 8989},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotIP, gotPort, err := SplitListenOn(tt.listenOn)
			require.NoError(t, err)
			require.Equal(t, tt.wantIP, gotIP)
			require.Equal(t, tt.wantPort, gotPort)
		})
	}
}

func TestConfig_ServerConfigs(t *testing.T) {
	t.Parallel()

	c := Config{ServerAddr: "http://127.0.0.1:8848,nacos:8848"}
	servers, err := c.ServerConfigs()
	require.NoError(t, err)
	require.Len(t, servers, 2)
	require.Equal(t, "127.0.0.1", servers[0].IpAddr)
	require.Equal(t, uint64(8848), servers[0].Port)
	require.Equal(t, "nacos", servers[1].IpAddr)
	require.Equal(t, uint64(8848), servers[1].Port)
}
