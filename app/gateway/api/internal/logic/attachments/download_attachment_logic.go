package attachments

import (
	"context"

	attachmentpb "github.com/hellopoisonx/aim/app/attachment/rpc/pb"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/membership"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type DownloadAttachmentLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDownloadAttachmentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DownloadAttachmentLogic {
	return &DownloadAttachmentLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DownloadAttachmentLogic) DownloadAttachment(req *types.DownloadAttachmentRequest) (resp *types.DownloadAttachmentResponse, err error) {
	userID, err := currentUserID(l.ctx)
	if err != nil {
		return nil, err
	}
	if l.svcCtx.AttachmentClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "attachment service unavailable")
	}

	// Attachments belong to a conversation; non-members must not download them.
	fileInfo, err := l.svcCtx.AttachmentClient.GetFile(l.ctx, &attachmentpb.GetFileReq{
		UserId: userID,
		FileId: req.Id,
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "get attachment", err)
	}

	if l.svcCtx.LogicConversationClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	if err := membership.RequireMember(l.ctx, l, l.svcCtx.LogicConversationClient, fileInfo.GetConversationId(), userID, membership.ActorUser); err != nil {
		return nil, err
	}

	rpcResp, err := l.svcCtx.AttachmentClient.AuthorizeDownload(l.ctx, &attachmentpb.AuthorizeDownloadReq{
		UserId: userID,
		FileId: req.Id,
	})
	if err != nil {
		return nil, err
	}

	return &types.DownloadAttachmentResponse{
		Url:       rpcResp.GetUrl(),
		Headers:   rpcResp.GetHeaders(),
		ExpiresAt: rpcResp.GetExpiresAt(),
	}, nil
}
