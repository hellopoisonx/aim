package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteBotWebhookLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewDeleteBotWebhookLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteBotWebhookLogic {
	return &DeleteBotWebhookLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// DeleteBotWebhook removes the bot's webhook configuration entirely.
func (l *DeleteBotWebhookLogic) DeleteBotWebhook(in *pb.DeleteBotWebhookReq) (*pb.DeleteBotWebhookResp, error) {
	botSvc, err := requireBot(l.svcCtx, in.GetBotUserId())
	if err != nil {
		return nil, err
	}

	deleted, err := botSvc.DeleteBotWebhook(l.ctx, in.GetBotUserId())
	if err != nil {
		return nil, errorx.NewCodeErrorf(errorx.CodeInternal, "delete bot webhook failed: %v", err)
	}

	return &pb.DeleteBotWebhookResp{Deleted: deleted}, nil
}
