package rocketmq

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/service"
)

func noopHandler(context.Context, string, string) error { return nil }

type noopService struct{}

func (noopService) Start() {}
func (noopService) Stop()  {}

func TestRegisterConsumerSkipsAbsentConfiguration(t *testing.T) {
	existing := []service.Service{noopService{}}

	services := RegisterConsumer(t.Context(), existing, "push", ConsumerConf{}, noopHandler)
	require.Equal(t, existing, services)
}

func TestRegisterConsumerKeepsServicesOnInvalidConfiguration(t *testing.T) {
	conf := ConsumerConf{EndpointConf: EndpointConf{Endpoint: "127.0.0.1:8081"}, Topic: "push.topic"}

	require.Empty(t, RegisterConsumer(t.Context(), nil, "push", conf, noopHandler))
}
