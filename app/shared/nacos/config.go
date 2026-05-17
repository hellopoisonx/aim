package nacos

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
)

const (
	defaultGroup       = "DEFAULT_GROUP"
	defaultCluster     = "DEFAULT"
	defaultWeight      = 1
	defaultTimeout     = 5 * time.Second
	defaultNacosPort   = 8848
	defaultNacosScheme = "http"
)

type Config struct {
	ServerAddr    string
	NamespaceID   string
	Group         string
	Cluster       string
	ServiceName   string
	AdvertiseIP   string
	AdvertisePort uint64
	Ephemeral     bool
	Weight        float64
	TimeoutMs     uint64
	Username      string
	Password      string
	LogDir        string
	CacheDir      string
	LogLevel      string
}

func (c *Config) ApplyDefaults(serviceName string, listenOn string) error {
	if c.ServerAddr == "" {
		c.ServerAddr = net.JoinHostPort("127.0.0.1", strconv.Itoa(defaultNacosPort))
	}

	if c.Group == "" {
		c.Group = defaultGroup
	}

	if c.Cluster == "" {
		c.Cluster = defaultCluster
	}

	if c.ServiceName == "" {
		c.ServiceName = serviceName
	}

	if c.Weight == 0 {
		c.Weight = defaultWeight
	}

	if c.TimeoutMs == 0 {
		c.TimeoutMs = uint64(defaultTimeout / time.Millisecond)
	}

	if c.LogLevel == "" {
		c.LogLevel = "info"
	}

	if c.LogDir == "" {
		c.LogDir = "logs/nacos"
	}

	if c.CacheDir == "" {
		c.CacheDir = "data/nacos"
	}

	if c.AdvertiseIP == "" || c.AdvertisePort == 0 {
		ip, port, err := SplitListenOn(listenOn)
		if err != nil {
			return err
		}

		if c.AdvertiseIP == "" {
			c.AdvertiseIP = ip
		}

		if c.AdvertisePort == 0 {
			c.AdvertisePort = port
		}
	}

	return nil
}

func (c Config) ServerConfigs() ([]constant.ServerConfig, error) {
	parts := strings.Split(c.ServerAddr, ",")

	servers := make([]constant.ServerConfig, 0, len(parts))
	for _, part := range parts {
		server, err := parseServer(strings.TrimSpace(part))
		if err != nil {
			return nil, err
		}

		servers = append(servers, server)
	}

	return servers, nil
}

func (c Config) ClientConfig() *constant.ClientConfig {
	return constant.NewClientConfig(
		constant.WithNamespaceId(c.NamespaceID),
		constant.WithTimeoutMs(c.TimeoutMs),
		constant.WithUsername(c.Username),
		constant.WithPassword(c.Password),
		constant.WithLogDir(c.LogDir),
		constant.WithCacheDir(c.CacheDir),
		constant.WithLogLevel(c.LogLevel),
	)
}

func SplitListenOn(listenOn string) (string, uint64, error) {
	host, portText, err := net.SplitHostPort(listenOn)
	if err != nil {
		return "", 0, fmt.Errorf("parse listen address %q: %w", listenOn, err)
	}

	port, err := strconv.ParseUint(portText, 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("parse listen port %q: %w", portText, err)
	}

	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}

	return host, port, nil
}

func parseServer(addr string) (constant.ServerConfig, error) {
	if addr == "" {
		return constant.ServerConfig{}, fmt.Errorf("nacos server address is empty")
	}

	scheme := defaultNacosScheme
	if before, after, ok := strings.Cut(addr, "://"); ok {
		scheme = before
		addr = after
	}

	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return constant.ServerConfig{}, fmt.Errorf("parse nacos server address %q: %w", addr, err)
	}

	port, err := strconv.ParseUint(portText, 10, 64)
	if err != nil {
		return constant.ServerConfig{}, fmt.Errorf("parse nacos server port %q: %w", portText, err)
	}

	return *constant.NewServerConfig(host, port, constant.WithScheme(scheme)), nil
}
