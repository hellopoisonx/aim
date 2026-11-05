package main

import (
	"context"
	"flag"

	"github.com/hellopoisonx/aim/app/attachment/internal/config"
	"github.com/hellopoisonx/aim/app/attachment/internal/server"
	attachmentservice "github.com/hellopoisonx/aim/app/attachment/internal/service"
	"github.com/hellopoisonx/aim/app/attachment/rpc/pb"
	rpcutil "github.com/hellopoisonx/aim/app/shared/rpc"
	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc"
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

	rpcutil.Run(c.RpcServerConf, func(grpcServer *grpc.Server) {
		pb.RegisterAttachmentServiceServer(grpcServer, server.NewAttachmentServiceServer(svc))
	}, nil)
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
	if c.Seaweed.DownloadTTLSeconds == 0 {
		c.Seaweed.DownloadTTLSeconds = 300
	}
	if c.UploadedProducer.Topic == "" {
		c.UploadedProducer.Topic = "aim-attachment-uploaded"
	}
}
