// Code scaffolded by goctl. Safe to edit.
// Source: logic.proto

package server

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
)

type MessageServiceServer struct {
	svcCtx *svc.ServiceContext
	pb.UnimplementedMessageServiceServer
}

func NewMessageServiceServer(svcCtx *svc.ServiceContext) *MessageServiceServer {
	return &MessageServiceServer{
		svcCtx: svcCtx,
	}
}

// IngestMessage is the single write entry for chat messages.
func (s *MessageServiceServer) IngestMessage(ctx context.Context, in *pb.IngestMessageReq) (*pb.IngestMessageResp, error) {
	ingestion := s.svcCtx.MessageService
	if ingestion == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "message service is not configured")
	}

	result, err := ingestion.IngestMessage(ctx, service.IngestMessageInput{
		SenderID:       in.GetSenderId(),
		DeviceID:       in.GetDeviceId(),
		ConversationID: in.GetConversationId(),
		MessageType:    in.GetMessageType(),
		Content:        in.GetContent(),
		ClientMsgID:    in.GetClientMsgId(),
		Mentions:       in.GetMentions(),
	})
	if err != nil {
		return nil, err
	}

	return &pb.IngestMessageResp{
		MessageId:   result.MessageID,
		ClientMsgId: result.ClientMsgID,
		AcceptedAt:  result.AcceptedAt,
	}, nil
}
