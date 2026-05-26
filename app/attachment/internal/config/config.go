package config

//lint:file-ignore SA5008 go-zero conf uses json tag options for defaults.

import (
	"time"

	"github.com/hellopoisonx/aim/app/shared/nacos"
	"github.com/zeromicro/go-zero/zrpc"
)

const MaxAttachmentSizeBytes = int64(5 * 1024 * 1024 * 1024)

type Config struct {
	zrpc.RpcServerConf
	Nacos    nacos.Config `json:",optional"`
	Postgres PostgresConf
	Seaweed  SeaweedConf
	Kafka    KqPusherConf `json:",optional"`
}

type PostgresConf struct {
	DataSource string `json:",optional"`
}

type SeaweedConf struct {
	Endpoint       string `json:",default=http://seaweed-s3:8333"`
	PublicEndpoint string `json:",default=http://localhost:8333"`
	Region         string `json:",default=us-east-1"`
	Bucket         string `json:",default=aim-attachments"`
	AccessKey      string `json:",optional"`
	SecretKey      string `json:",optional"`
	PresignTTL     int    `json:",default=900"`
	DownloadTTL    int    `json:",default=300"`
	MaxSizeBytes   int64  `json:",default=5368709120"`
	ForcePathStyle bool   `json:",default=true"`
}

type KqPusherConf struct {
	Brokers []string `json:",optional"`
	Topic   string   `json:",default=aim.attachment.uploaded"`
}

func (c SeaweedConf) UploadTTL() time.Duration {
	if c.PresignTTL <= 0 {
		return 15 * time.Minute
	}
	return time.Duration(c.PresignTTL) * time.Second
}

func (c SeaweedConf) GetDownloadTTL() time.Duration {
	if c.DownloadTTL <= 0 {
		return 5 * time.Minute
	}
	return time.Duration(c.DownloadTTL) * time.Second
}

func (c SeaweedConf) MaxSize() int64 {
	if c.MaxSizeBytes <= 0 {
		return MaxAttachmentSizeBytes
	}
	if c.MaxSizeBytes > MaxAttachmentSizeBytes {
		return MaxAttachmentSizeBytes
	}
	return c.MaxSizeBytes
}
