package svc

import (
	"testing"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/config"

	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestNewServiceContextRequiresPostgres(t *testing.T) {
	logx.ExitOnFatal.Set(false)
	t.Cleanup(func() { logx.ExitOnFatal.Set(true) })

	require.Panics(t, func() { NewServiceContext(config.Config{MachineID: 1}) })
}
