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
	require.Equal(t, "nacos:8848", c.Nacos.ServerAddr)
}
