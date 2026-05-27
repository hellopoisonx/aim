package attachments

import (
	"context"

	attachmentpb "github.com/hellopoisonx/aim/app/attachment/rpc/pb"
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
