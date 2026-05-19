package nacos

import (
	"fmt"
	"math/rand"
	"sync"

	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/resolver"
)

// Scheme is the resolver scheme for Nacos-based gRPC service discovery.
const Scheme = "nacos"

// maxSubsetSize caps the number of addresses passed to gRPC load balancing.
const maxSubsetSize = 32

var registerOnce sync.Once

// ResolverBuilder implements gRPC resolver.Builder backed by Nacos subscription.
// It watches Nacos for instance changes and updates gRPC's address list dynamically,
// so the gateway no longer panics when auth registers after gateway startup.
type ResolverBuilder struct {
	Client NamingClient
	Config Config
}

// Build creates a Resolver for the given target.
func (b *ResolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, _ resolver.BuildOptions) (resolver.Resolver, error) {
	r := &nacosResolver{
		client: b.Client,
		cc:     cc,
	}

	param := &vo.SubscribeParam{
		ServiceName: b.Config.ServiceName,
		GroupName:   b.Config.Group,
		Clusters:    []string{b.Config.Cluster},
		SubscribeCallback: func(instances []model.Instance, err error) {
			if err != nil {
				logx.Errorf("nacos subscribe callback error for %q: %v", b.Config.ServiceName, err)
				return
			}

			r.updateAddrs(instances)
		},
	}

	if err := b.Client.Subscribe(param); err != nil {
		return nil, fmt.Errorf("nacos subscribe %q: %w", b.Config.ServiceName, err)
	}

	r.unsubscribe = func() {
		_ = b.Client.Unsubscribe(param)
	}

	// Initial fetch — may be empty if auth hasn't started yet, which is fine.
	instances, err := b.Client.SelectInstances(vo.SelectInstancesParam{
		ServiceName: b.Config.ServiceName,
		GroupName:   b.Config.Group,
		Clusters:    []string{b.Config.Cluster},
		HealthyOnly: true,
	})
	if err != nil {
		logx.Errorf("nacos initial SelectInstances for %q: %v", b.Config.ServiceName, err)
	} else {
		r.updateAddrs(instances)
	}

	return r, nil
}

// Scheme returns the resolver scheme.
func (b *ResolverBuilder) Scheme() string {
	return Scheme
}

type nacosResolver struct {
	client      NamingClient
	cc          resolver.ClientConn
	unsubscribe func()
	mu          sync.Mutex
}

func (r *nacosResolver) updateAddrs(instances []model.Instance) {
	r.mu.Lock()
	defer r.mu.Unlock()

	addrs := make([]resolver.Address, 0, len(instances))
	for _, inst := range instances {
		if !inst.Enable || !inst.Healthy || inst.Weight <= 0 {
			continue
		}

		addrs = append(addrs, resolver.Address{Addr: fmt.Sprintf("%s:%d", inst.Ip, inst.Port)})
	}

	addrs = subset(addrs, maxSubsetSize)

	if err := r.cc.UpdateState(resolver.State{Addresses: addrs}); err != nil {
		logx.Errorf("nacos resolver UpdateState: %v", err)
	}
}

func (r *nacosResolver) ResolveNow(resolver.ResolveNowOptions) {}

func (r *nacosResolver) Close() {
	if r.unsubscribe != nil {
		r.unsubscribe()
	}
}

// subset shuffles and caps the address list for consistent load distribution.
func subset[T any](set []T, n int) []T {
	rand.Shuffle(len(set), func(i, j int) {
		set[i], set[j] = set[j], set[i]
	})

	if len(set) <= n {
		return set
	}

	return set[:n]
}

// RegisterResolver registers the Nacos-backed gRPC resolver globally.
// Must be called before any grpc.Dial / zrpc.NewClientWithTarget that uses the "nacos" scheme.
// Safe to call multiple times; only the first registration takes effect.
func RegisterResolver(client NamingClient, c Config) {
	registerOnce.Do(func() {
		resolver.Register(&ResolverBuilder{Client: client, Config: c})
	})
}
