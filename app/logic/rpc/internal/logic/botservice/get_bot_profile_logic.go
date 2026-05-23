package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetBotProfileLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetBotProfileLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetBotProfileLogic {
	return &GetBotProfileLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// GetBotProfile returns the bot's user_info snapshot for `/api/bot/v1/me`.
func (l *GetBotProfileLogic) GetBotProfile(in *pb.GetBotProfileReq) (*pb.GetBotProfileResp, error) {
	if l.svcCtx.BotService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}

	if in.GetBotUserId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "bot_user_id is required")
	}

	info, err := l.svcCtx.BotService.GetBotProfile(l.ctx, in.GetBotUserId())
	if err != nil {
		return nil, service.ToGRPCError(err)
	}

	return &pb.GetBotProfileResp{
		Profile: &pb.UserInfoResponse{
			Id:        info.ID,
			Email:     info.Email,
			Status:    int32(info.Status),
			Nickname:  info.Nickname,
			Avatar:    info.Avatar,
			CreatedAt: info.CreatedAt.Time.UnixMilli(),
			UpdatedAt: info.UpdatedAt.Time.UnixMilli(),
		},
	}, nil
}
