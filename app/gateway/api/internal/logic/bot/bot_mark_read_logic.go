package bot

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/conversationservice"
	"github.com/hellopoisonx/aim/app/shared/botperm"

	"github.com/zeromicro/go-zero/core/logx"
)

type BotMarkReadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewBotMarkReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BotMarkReadLogic {
	return &BotMarkReadLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *BotMarkReadLogic) BotMarkRead(req *types.BotMarkReadRequest) (*types.BotMarkReadResponse, error) {
	identity, err := requireBotAction(l.ctx, botperm.ActionReadReceiptWrite)
	if err != nil {
		return nil, err
	}

	conversationID, err := parseRequiredID(req.Id, "id")
	if err != nil {
		return nil, err
	}

	lastReadMessageID, err := parseRequiredID(req.LastReadMessageId, "last_read_message_id")
	if err != nil {
		return nil, err
	}

	client, err := requireLogicConversationClient(l.svcCtx)
	if err != nil {
		return nil, err
	}

	rpcResp, err := client.UpdateReadReceipt(l.ctx, &conversationservice.UpdateReadReceiptReq{
		ConversationId:    conversationID,
		UserId:            identity.BotUserID,
		LastReadMessageId: lastReadMessageID,
	})
	if err != nil {
		return nil, sanitizeBotLogicRPCError(err)
	}

	readState := rpcResp.GetReadState()
	publishBotReadReceipt(l.ctx, l.svcCtx, identity, conversationID, readState)

	return &types.BotMarkReadResponse{ReadState: convertBotReadState(readState)}, nil
}
