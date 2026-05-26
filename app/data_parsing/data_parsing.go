package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/hellopoisonx/aim/app/data_parsing/internal/config"
	"github.com/hellopoisonx/aim/app/data_parsing/internal/worker"
	"github.com/zeromicro/go-queue/kq"
	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	zservice "github.com/zeromicro/go-zero/core/service"
)

var configFile = flag.String("f", "etc/data_parsing.yaml", "the config file")

func main() {
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c)
	applyDefaults(&c)
	zservice.ServiceConf{Name: c.Name, Telemetry: c.Telemetry}.MustSetUp()

	ctx := context.Background()
	w, err := worker.New(ctx, c)
	logx.Must(err)
	defer w.Close()

	group := zservice.NewServiceGroup()
	group.Add(kq.MustNewQueue(c.UploadedConsumer, w))
	defer group.Stop()
	fmt.Println("Starting data_parsing service...")
	group.Start()
}

func applyDefaults(c *config.Config) {
	if c.Name == "" {
		c.Name = "data_parsing"
	}
	if c.Telemetry.Name == "" {
		c.Telemetry.Name = c.Name
	}
	if c.Seaweed.Endpoint == "" {
		c.Seaweed.Endpoint = "http://seaweed-s3:8333"
	}
	if c.Seaweed.Bucket == "" {
		c.Seaweed.Bucket = "aim-attachments"
	}
	if c.ParsedProducer.Topic == "" {
		c.ParsedProducer.Topic = "aim.attachment.parsed"
	}
}
