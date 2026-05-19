package nacos

import (
	"testing"

	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"github.com/stretchr/testify/require"
)

type fakeNamingClient struct {
	registerParam    vo.RegisterInstanceParam
	deregisterParam  vo.DeregisterInstanceParam
	instances        []model.Instance
	subscribeCB      func(services []model.Instance, err error)
	subscribeErr     error
	unsubscribeCount int
}

func (f *fakeNamingClient) RegisterInstance(param vo.RegisterInstanceParam) (bool, error) {
	f.registerParam = param
	return true, nil
}

func (f *fakeNamingClient) DeregisterInstance(param vo.DeregisterInstanceParam) (bool, error) {
	f.deregisterParam = param
	return true, nil
}

func (f *fakeNamingClient) SelectInstances(vo.SelectInstancesParam) ([]model.Instance, error) {
	return f.instances, nil
}

func (f *fakeNamingClient) Subscribe(param *vo.SubscribeParam) error {
	if f.subscribeErr != nil {
		return f.subscribeErr
	}

	f.subscribeCB = param.SubscribeCallback

	return nil
}

func (f *fakeNamingClient) Unsubscribe(_ *vo.SubscribeParam) error {
	f.unsubscribeCount++
	return nil
}

func (f *fakeNamingClient) CloseClient() {}

func TestRegisterAndDeregisterInstance(t *testing.T) {
	t.Parallel()

	client := &fakeNamingClient{}
	c := Config{
		Group:         "AIM",
		Cluster:       "BJ",
		ServiceName:   "auth.rpc",
		AdvertiseIP:   "127.0.0.1",
		AdvertisePort: 8080,
		Ephemeral:     true,
		Weight:        2,
	}

	require.NoError(t, RegisterInstance(client, c))
	require.Equal(t, "auth.rpc", client.registerParam.ServiceName)
	require.Equal(t, "AIM", client.registerParam.GroupName)
	require.Equal(t, "BJ", client.registerParam.ClusterName)
	require.Equal(t, uint64(8080), client.registerParam.Port)
	require.InEpsilon(t, 2, client.registerParam.Weight, 0)

	require.NoError(t, DeregisterInstance(client, c))
	require.Equal(t, "auth.rpc", client.deregisterParam.ServiceName)
	require.Equal(t, "AIM", client.deregisterParam.GroupName)
	require.Equal(t, "BJ", client.deregisterParam.Cluster)
}

func TestBuildDirectTarget(t *testing.T) {
	t.Parallel()

	client := &fakeNamingClient{instances: []model.Instance{
		{Ip: "127.0.0.1", Port: 8080, Healthy: true, Enable: true, Weight: 1},
		{Ip: "127.0.0.2", Port: 8081, Healthy: false, Enable: true, Weight: 1},
		{Ip: "127.0.0.3", Port: 8082, Healthy: true, Enable: true, Weight: 0},
	}}
	c := Config{Group: "AIM", Cluster: "BJ", ServiceName: "auth.rpc"}

	target, err := BuildDirectTarget(client, c)
	require.NoError(t, err)
	require.Equal(t, "direct:///127.0.0.1:8080", target)
}

func TestBuildDirectTargetNoHealthyInstance(t *testing.T) {
	t.Parallel()

	client := &fakeNamingClient{}
	_, err := BuildDirectTarget(client, Config{ServiceName: "auth.rpc"})
	require.Error(t, err)
}
