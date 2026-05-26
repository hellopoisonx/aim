package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hellopoisonx/aim/app/attachment/internal/config"
	"github.com/hellopoisonx/aim/app/attachment/internal/server"
	attachmentservice "github.com/hellopoisonx/aim/app/attachment/internal/service"
	"github.com/hellopoisonx/aim/app/attachment/rpc/pb"
	aimnacos "github.com/hellopoisonx/aim/app/shared/nacos"
	rpcutil "github.com/hellopoisonx/aim/app/shared/rpc"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
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
	logx.Must(c.Nacos.ApplyDefaults(c.Name, c.ListenOn))

	namingClient, err := aimnacos.NewNamingClient(c.Nacos)
	logx.Must(err)

	logx.Must(aimnacos.RegisterInstance(namingClient, c.Nacos))
	defer namingClient.CloseClient()
	defer func() {
		if err := aimnacos.DeregisterInstance(namingClient, c.Nacos); err != nil {
			logx.Error(err)
		}
	}()

	ctx := context.Background()
	if err := runMigrations(ctx, c.Postgres.DataSource, "app/attachment/model/migrations"); err != nil {
		logx.Errorf("attachment migration failed: %v", err)
	}

	svc, err := attachmentservice.New(ctx, c)
	logx.Must(err)
	defer svc.Close()

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

func runMigrations(ctx context.Context, dsn, dir string) error {
	if dsn == "" {
		return nil
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	entries, err := os.ReadDir(dir)
	if err != nil {
		// Container working directory is /app, where migrations are copied under /app/model/migrations.
		alt := filepath.Join("model", "migrations")
		entries, err = os.ReadDir(alt)
		if err != nil {
			return nil
		}
		dir = alt
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		// #nosec G304 -- migration file names come from os.ReadDir on a trusted migrations directory.
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, string(b)); err != nil {
			return err
		}
	}
	return nil
}
