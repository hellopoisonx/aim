package rpc

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/zrpc"
)

func TestConfigured(t *testing.T) {
	require.False(t, Configured(zrpc.RpcClientConf{}))
	require.True(t, Configured(zrpc.RpcClientConf{Target: "127.0.0.1:9000"}))
	require.True(t, Configured(zrpc.RpcClientConf{Endpoints: []string{"127.0.0.1:9000"}}))

	withEtcd := zrpc.RpcClientConf{}
	withEtcd.Etcd.Key = "logic.rpc"
	require.True(t, Configured(withEtcd))
}

func TestDialOptionalReturnsNilWhenUnconfigured(t *testing.T) {
	require.Nil(t, DialOptional(t.Context(), "AttachmentRpc", zrpc.RpcClientConf{}))
}
