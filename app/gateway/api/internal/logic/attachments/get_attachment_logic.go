package attachments

import (
	"context"

	attachmentpb "github.com/hellopoisonx/aim/app/attachment/rpc/pb"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetAttachmentLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetAttachmentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetAttachmentLogic {
	return &GetAttachmentLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetAttachmentLogic) GetAttachment(req *types.GetAttachmentRequest) (resp *types.AttachmentFileInfo, err error) {
	userID, err := currentUserID(l.ctx)
	if err != nil {
		return nil, err
	}
	if l.svcCtx.AttachmentClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "attachment service unavailable")
	}

	info, err := l.svcCtx.AttachmentClient.GetFile(l.ctx, &attachmentpb.GetFileReq{
		UserId: userID,
		FileId: req.Id,
	})
	if err != nil {
		return nil, err
	}

	return fileInfoFromPB(info), nil
}
