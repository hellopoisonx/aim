package bot

import (
	"context"
	"time"

	"github.com/hellopoisonx/aim/app/core/rpc/pb"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/botctx"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/shared/botperm"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

// botDeviceID is the device_id reported to core.Transfer for Bot OpenAPI
// requests. It keeps Bot quotas separate from human WS quotas via the
// (sender_id, device_id) key tuple in TransferLogic.
const botDeviceID = "bot-api"

// botSendAction is the token action required to publish a message.
const botSendAction = botperm.ActionMessageSend

type BotSendMessageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewBotSendMessageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BotSendMessageLogic {
	return &BotSendMessageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *BotSendMessageLogic) BotSendMessage(req *types.BotSendMessageRequest) (*types.BotSendMessageResponse, error) {
	identity, ok := botctx.FromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeBotTokenInvalid, "missing bot identity")
	}

	if err := identity.RequireAction(botSendAction); err != nil {
		return nil, err
	}

	if req.ConversationId <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "conversation_id is required")
	}

	if req.MessageType == "" {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "message_type is required")
	}

	if req.ClientMsgId == "" {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "client_msg_id is required")
	}

	if l.svcCtx.CoreClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	// Match the WS handler timeout so REST and WS surfaces behave the same.
	ctx, cancel := context.WithTimeout(l.ctx, 3*time.Second)
	defer cancel()

	rpcResp, err := l.svcCtx.CoreClient.Transfer(ctx, &pb.TransferReq{
		SenderId:       identity.BotUserID,
		DeviceId:       botDeviceID,
		ConversationId: req.ConversationId,
		MessageType:    req.MessageType,
		Content:        req.Content,
		ClientMsgId:    req.ClientMsgId,
		Mentions:       req.Mentions,
	})
	if err != nil {
		if ce := errorx.FromGRPCError(err); ce != nil {
			return nil, ce
		}
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	return &types.BotSendMessageResponse{
		MessageId:   rpcResp.GetMessageId(),
		ClientMsgId: rpcResp.GetClientMsgId(),
		AcceptedAt:  rpcResp.GetAcceptedAt(),
	}, nil
}
