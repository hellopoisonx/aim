package config

//lint:file-ignore SA5008 go-zero conf uses json tag options for defaults.

import (
	"github.com/hellopoisonx/aim/app/shared/rocketmq"

	"github.com/zeromicro/go-zero/core/prometheus"
	"github.com/zeromicro/go-zero/core/trace"
)

type Config struct {
	Name              string            `json:",default=data_parsing"`
	Prometheus        prometheus.Config `json:",optional"`
	Telemetry         trace.Config      `json:",optional"`
	Postgres          PostgresConf
	Seaweed           SeaweedConf
	UploadedConsumer  rocketmq.ConsumerConf `json:",optional"`
	ParsedProducer    rocketmq.ProducerConf `json:",optional"`
	ThumbnailMaxWidth int                   `json:",default=320"`
}

type PostgresConf struct {
	DataSource string `json:",optional"`
}

type SeaweedConf struct {
	Endpoint  string `json:",default=http://seaweed-s3:8333"`
	Bucket    string `json:",default=aim-attachments"`
	AccessKey string `json:",optional"`
	SecretKey string `json:",optional"`
}
