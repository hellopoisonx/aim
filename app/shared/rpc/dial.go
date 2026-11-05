package rpc

import (
	"context"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
)

// Configured reports whether conf names a way to reach the service: an etcd
// registry key, static endpoints, or a direct target.
func Configured(conf zrpc.RpcClientConf) bool {
	return conf.Etcd.Key != "" || len(conf.Endpoints) > 0 || conf.Target != ""
}

// DialOptional creates a client for an optional dependency. An absent
// configuration or a failed dial is logged and yields nil, so the caller
// degrades instead of aborting startup.
func DialOptional(ctx context.Context, label string, conf zrpc.RpcClientConf) zrpc.Client {
	if !Configured(conf) {
		return nil
	}
	client, err := zrpc.NewClient(conf)
	if err != nil {
		logx.WithContext(ctx).Errorf("failed to create RPC client for %s: %v", label, err)
		return nil
	}
	return client
}
