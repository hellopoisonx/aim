package main

import (
	"net/http"
	"testing"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/config"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/handler"
	handlersws "github.com/hellopoisonx/aim/app/gateway/api/internal/handler/ws"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/rest"
)

func TestRegisterHandlersIncludesWebSocketRoute(t *testing.T) {
	t.Parallel()

	var c config.Config
	c.Auth.AccessSecret = "test-secret"
	server := rest.MustNewServer(c.RestConf)
	defer server.Stop()

	svcCtx := svc.NewServiceContextWithAuth(c, nil)
	handler.RegisterHandlers(server, svcCtx)
	wsHandler := handlersws.NewWsHandler(svcCtx, svcCtx.WsManager)
	server.AddRoute(rest.Route{Method: http.MethodGet, Path: "/ws", Handler: wsHandler.ServeWS})

	assertHasRoute(t, server.Routes(), http.MethodGet, "/ws")
}

func assertHasRoute(t *testing.T, routes []rest.Route, method, path string) {
	t.Helper()

	for _, route := range routes {
		if route.Method == method && route.Path == path {
			return
		}
	}

	require.Failf(t, "route missing", "expected route %s %s to be registered", method, path)
}
