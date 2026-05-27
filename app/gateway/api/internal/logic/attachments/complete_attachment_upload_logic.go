package attachments

import (
	"context"

	attachmentpb "github.com/hellopoisonx/aim/app/attachment/rpc/pb"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type CompleteAttachmentUploadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCompleteAttachmentUploadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CompleteAttachmentUploadLogic {
	return &CompleteAttachmentUploadLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CompleteAttachmentUploadLogic) CompleteAttachmentUpload(req *types.CompleteAttachmentUploadRequest) (resp *types.AttachmentFileInfo, err error) {
	userID, err := currentUserID(l.ctx)
	if err != nil {
		return nil, err
	}
	if l.svcCtx.AttachmentClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "attachment service unavailable")
	}

	info, err := l.svcCtx.AttachmentClient.CompleteUpload(l.ctx, &attachmentpb.CompleteUploadReq{
		UserId: userID,
		FileId: req.Id,
		Sha256: req.Sha256,
	})
	if err != nil {
		return nil, err
	}

	return fileInfoFromPB(info), nil
}
