package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/hellopoisonx/aim/app/attachment/internal/config"
	"github.com/hellopoisonx/aim/app/attachment/internal/server"
	attachmentservice "github.com/hellopoisonx/aim/app/attachment/internal/service"
	"github.com/hellopoisonx/aim/app/attachment/rpc/pb"
	rpcutil "github.com/hellopoisonx/aim/app/shared/rpc"
	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/proc"
	zservice "github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/attachment.yaml", "the config file")

func main() {
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c)
	applyDefaults(&c)

	ctx := context.Background()

	svc, err := attachmentservice.New(ctx, c)
	logx.Must(err)
	defer svc.Close()

	// Start outbox poller if configured
	if poller := svc.OutboxPoller(); poller != nil {
		poller.Start(context.Background())
		proc.AddWrapUpListener(func() {
			poller.Stop()
		})
	}

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		pb.RegisterAttachmentServiceServer(grpcServer, server.NewAttachmentServiceServer(svc))

		if c.Mode == zservice.DevMode || c.Mode == zservice.TestMode {
			reflection.Register(grpcServer)
		}
	})
	s.AddUnaryInterceptors(rpcutil.UnaryErrorInterceptor())
	defer s.Stop()

	fmt.Printf("Starting attachment rpc server at %s...\n", c.ListenOn)
	s.Start()
}

func applyDefaults(c *config.Config) {
	if c.Name == "" {
		c.Name = "attachment.rpc"
	}
	if c.ListenOn == "" {
		c.ListenOn = "0.0.0.0:8091"
	}
	if c.Telemetry.Name == "" {
		c.Telemetry.Name = c.Name
	}
	if c.Seaweed.Endpoint == "" {
		c.Seaweed.Endpoint = "http://seaweed-s3:8333"
	}
	if c.Seaweed.PublicEndpoint == "" {
		c.Seaweed.PublicEndpoint = "http://localhost:8333"
	}
	if c.Seaweed.Region == "" {
		c.Seaweed.Region = "us-east-1"
	}
	if c.Seaweed.Bucket == "" {
		c.Seaweed.Bucket = "aim-attachments"
	}
	if c.Seaweed.PresignTTL == 0 {
		c.Seaweed.PresignTTL = 900
	}
	if c.Seaweed.DownloadTTL == 0 {
		c.Seaweed.DownloadTTL = 300
	}
	if c.Kafka.Topic == "" {
		c.Kafka.Topic = "aim.attachment.uploaded"
	}
}
