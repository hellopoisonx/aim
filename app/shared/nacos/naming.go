package nacos

import (
	"fmt"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"github.com/zeromicro/go-zero/zrpc/resolver"
)

type NamingClient interface {
	RegisterInstance(param vo.RegisterInstanceParam) (bool, error)
	DeregisterInstance(param vo.DeregisterInstanceParam) (bool, error)
	SelectInstances(param vo.SelectInstancesParam) ([]model.Instance, error)
	Subscribe(param *vo.SubscribeParam) error
	Unsubscribe(param *vo.SubscribeParam) error
	CloseClient()
}

func NewNamingClient(c Config) (NamingClient, error) {
	servers, err := c.ServerConfigs()
	if err != nil {
		return nil, err
	}

	return clients.NewNamingClient(vo.NacosClientParam{
		ClientConfig:  c.ClientConfig(),
		ServerConfigs: servers,
	})
}

func RegisterInstance(client NamingClient, c Config) error {
	ok, err := client.RegisterInstance(vo.RegisterInstanceParam{
		Ip:          c.AdvertiseIP,
		Port:        c.AdvertisePort,
		Weight:      c.Weight,
		Enable:      true,
		Healthy:     true,
		ClusterName: c.Cluster,
		ServiceName: c.ServiceName,
		GroupName:   c.Group,
		Ephemeral:   c.Ephemeral,
	})
	if err != nil {
		return err
	}

	if !ok {
		return fmt.Errorf("register nacos service %q returned false", c.ServiceName)
	}

	return nil
}

func DeregisterInstance(client NamingClient, c Config) error {
	ok, err := client.DeregisterInstance(vo.DeregisterInstanceParam{
		Ip:          c.AdvertiseIP,
		Port:        c.AdvertisePort,
		Cluster:     c.Cluster,
		ServiceName: c.ServiceName,
		GroupName:   c.Group,
		Ephemeral:   c.Ephemeral,
	})
	if err != nil {
		return err
	}

	if !ok {
		return fmt.Errorf("deregister nacos service %q returned false", c.ServiceName)
	}

	return nil
}

func BuildDirectTarget(client NamingClient, c Config) (string, error) {
	instances, err := client.SelectInstances(vo.SelectInstancesParam{
		Clusters:    []string{c.Cluster},
		ServiceName: c.ServiceName,
		GroupName:   c.Group,
		HealthyOnly: true,
	})
	if err != nil {
		return "", err
	}

	endpoints := make([]string, 0, len(instances))
	for _, instance := range instances {
		if !instance.Enable || !instance.Healthy || instance.Weight <= 0 {
			continue
		}

		endpoints = append(endpoints, fmt.Sprintf("%s:%d", instance.Ip, instance.Port))
	}

	if len(endpoints) == 0 {
		return "", fmt.Errorf("no healthy nacos instances found for service %q", c.ServiceName)
	}

	return resolver.BuildDirectTarget(endpoints), nil
}
