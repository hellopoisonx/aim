package nacos

import (
	"testing"

	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/serviceconfig"
)

type fakeClientConn struct {
	state resolver.State
}

func (c *fakeClientConn) UpdateState(state resolver.State) error {
	c.state = state
	return nil
}

func (c *fakeClientConn) ReportError(err error)                                  {}
func (c *fakeClientConn) NewAddress(_ []resolver.Address)                        {}
func (c *fakeClientConn) ParseServiceConfig(_ string) *serviceconfig.ParseResult { return nil }

var _ resolver.ClientConn = (*fakeClientConn)(nil)

func TestResolverBuilder_Scheme(t *testing.T) {
	t.Parallel()

	b := &ResolverBuilder{}
	assert.Equal(t, "nacos", b.Scheme())
}

func TestResolverBuilder_Build_EmptyInstances(t *testing.T) {
	t.Parallel()

	client := &fakeNamingClient{}
	cfg := Config{Group: "AIM", Cluster: "BJ", ServiceName: "auth.rpc"}
	builder := &ResolverBuilder{Client: client, Config: cfg}
	cc := &fakeClientConn{}

	r, err := builder.Build(resolver.Target{}, cc, resolver.BuildOptions{})
	require.NoError(t, err)
	require.NotNil(t, r)

	// No instances yet — address list should be empty, NOT a panic.
	assert.Empty(t, cc.state.Addresses)

	// Verify subscription was started.
	require.NotNil(t, client.subscribeCB)

	r.Close()
	assert.Equal(t, 1, client.unsubscribeCount)
}

func TestResolverBuilder_Build_WithHealthyInstances(t *testing.T) {
	t.Parallel()

	client := &fakeNamingClient{instances: []model.Instance{
		{Ip: "10.0.0.1", Port: 8080, Healthy: true, Enable: true, Weight: 1},
		{Ip: "10.0.0.2", Port: 8081, Healthy: false, Enable: true, Weight: 1}, // unhealthy
		{Ip: "10.0.0.3", Port: 8082, Healthy: true, Enable: true, Weight: 0},  // weight 0
	}}
	cfg := Config{Group: "AIM", Cluster: "BJ", ServiceName: "auth.rpc"}
	builder := &ResolverBuilder{Client: client, Config: cfg}
	cc := &fakeClientConn{}

	r, err := builder.Build(resolver.Target{}, cc, resolver.BuildOptions{})
	require.NoError(t, err)
	require.NotNil(t, r)

	// Only 10.0.0.1:8080 is healthy, enabled, and weight>0.
	assert.Len(t, cc.state.Addresses, 1)
	assert.Equal(t, "10.0.0.1:8080", cc.state.Addresses[0].Addr)

	r.Close()
}

func TestResolverBuilder_SubscribeCallback_AddsInstances(t *testing.T) {
	t.Parallel()

	client := &fakeNamingClient{} // starts empty
	cfg := Config{Group: "AIM", Cluster: "BJ", ServiceName: "auth.rpc"}
	builder := &ResolverBuilder{Client: client, Config: cfg}
	cc := &fakeClientConn{}

	r, err := builder.Build(resolver.Target{}, cc, resolver.BuildOptions{})
	require.NoError(t, err)
	assert.Empty(t, cc.state.Addresses)

	// Simulate auth registering — Nacos callback fires.
	client.subscribeCB([]model.Instance{
		{Ip: "10.0.0.5", Port: 9090, Healthy: true, Enable: true, Weight: 2},
	}, nil)

	assert.Len(t, cc.state.Addresses, 1)
	assert.Equal(t, "10.0.0.5:9090", cc.state.Addresses[0].Addr)

	r.Close()
}

func TestResolverBuilder_SubscribeCallback_RemovesInstances(t *testing.T) {
	t.Parallel()

	client := &fakeNamingClient{instances: []model.Instance{
		{Ip: "10.0.0.1", Port: 8080, Healthy: true, Enable: true, Weight: 1},
	}}
	cfg := Config{Group: "AIM", Cluster: "BJ", ServiceName: "auth.rpc"}
	builder := &ResolverBuilder{Client: client, Config: cfg}
	cc := &fakeClientConn{}

	r, err := builder.Build(resolver.Target{}, cc, resolver.BuildOptions{})
	require.NoError(t, err)
	assert.Len(t, cc.state.Addresses, 1)

	// Simulate auth going unhealthy.
	client.subscribeCB([]model.Instance{
		{Ip: "10.0.0.1", Port: 8080, Healthy: false, Enable: true, Weight: 1},
	}, nil)

	assert.Empty(t, cc.state.Addresses)

	r.Close()
}

func TestRegisterResolver(t *testing.T) {
	client := &fakeNamingClient{instances: []model.Instance{
		{Ip: "127.0.0.1", Port: 8080, Healthy: true, Enable: true, Weight: 1},
	}}
	cfg := Config{Group: "AIM", Cluster: "BJ", ServiceName: "auth.rpc"}

	RegisterResolver(client, cfg)

	rb := resolver.Get("nacos")
	require.NotNil(t, rb, "nacos resolver should be registered")
	assert.Equal(t, "nacos", rb.Scheme())
}
