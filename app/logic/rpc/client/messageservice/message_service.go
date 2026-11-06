// Code scaffolded by goctl. Safe to edit.
// Source: logic.proto

package messageservice

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/pb"

	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
)

type (
	IngestMessageReq  = pb.IngestMessageReq
	IngestMessageResp = pb.IngestMessageResp

	MessageService interface {
		// IngestMessage is the single write entry for chat messages.
		IngestMessage(ctx context.Context, in *IngestMessageReq, opts ...grpc.CallOption) (*IngestMessageResp, error)
	}

	defaultMessageService struct {
		cli zrpc.Client
	}
)

func NewMessageService(cli zrpc.Client) MessageService {
	return &defaultMessageService{
		cli: cli,
	}
}

func (m *defaultMessageService) IngestMessage(ctx context.Context, in *IngestMessageReq, opts ...grpc.CallOption) (*IngestMessageResp, error) {
	client := pb.NewMessageServiceClient(m.cli.Conn())
	return client.IngestMessage(ctx, in, opts...)
}
