package userbots

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/botservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
)

// setUserBotStatus flips a bot's enabled flag for its owner and returns the new state.
func setUserBotStatus(
	ctx context.Context,
	svcCtx *svc.ServiceContext,
	logger errorx.ErrorLogger,
	operation string,
	botUserID int64,
	enabled bool,
) (*pb.UserBotInfo, error) {
	ownerUserID, err := authctx.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}

	rpcResp, err := svcCtx.LogicBotClient.SetUserBotStatus(ctx, &botservice.SetUserBotStatusReq{
		OwnerUserId: ownerUserID, BotUserId: botUserID, Enabled: enabled,
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(logger, operation, err)
	}

	return rpcResp.GetBot(), nil
}
