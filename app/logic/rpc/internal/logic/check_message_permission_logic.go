package logic

import (
	"context"
	"errors"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"

	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var errPermissionCheckerMissing = errors.New("permission checker is not configured")

const (
	maxMessageTypeLength = 32
	maxMentions          = 20
)

type CheckMessagePermissionLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCheckMessagePermissionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CheckMessagePermissionLogic {
	return &CheckMessagePermissionLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// CheckMessagePermission checks whether a sender can publish into a conversation.
func (l *CheckMessagePermissionLogic) CheckMessagePermission(in *pb.CheckMessagePermissionReq) (*pb.CheckMessagePermissionResp, error) {
	if in.GetSenderId() <= 0 || in.GetConversationId() <= 0 {
		return &pb.CheckMessagePermissionResp{
			Allowed: false,
			BizCode: service.CodeInvalidArgument,
			Reason:  "sender_id and conversation_id are required",
		}, nil
	}

	if len(in.GetMessageType()) > maxMessageTypeLength {
		return &pb.CheckMessagePermissionResp{
			Allowed: false,
			BizCode: service.CodeInvalidArgument,
			Reason:  "message_type is too long",
		}, nil
	}

	if len(in.GetMentions()) > maxMentions {
		return &pb.CheckMessagePermissionResp{
			Allowed: false,
			BizCode: service.CodeInvalidArgument,
			Reason:  "mentions exceeds limit",
		}, nil
	}

	for _, mention := range in.GetMentions() {
		if mention <= 0 {
			return &pb.CheckMessagePermissionResp{
				Allowed: false,
				BizCode: service.CodeInvalidArgument,
				Reason:  "mentions must contain positive user ids",
			}, nil
		}
	}

	checker := l.svcCtx.PermissionChecker
	if checker == nil {
		return nil, status.Error(codes.Internal, errPermissionCheckerMissing.Error())
	}

	decision, err := checker.CheckMessagePermission(l.ctx, service.PermissionCheck{
		SenderID:       in.GetSenderId(),
		ConversationID: in.GetConversationId(),
		MessageType:    in.GetMessageType(),
		Mentions:       in.GetMentions(),
	})
	if err != nil {
		return nil, status.Error(codes.Internal, "check message permission failed")
	}

	return &pb.CheckMessagePermissionResp{
		Allowed: decision.Allowed,
		BizCode: decision.Code,
		Reason:  decision.Reason,
	}, nil
}
