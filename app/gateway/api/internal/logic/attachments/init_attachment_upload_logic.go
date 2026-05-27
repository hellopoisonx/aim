package attachments

import (
	"context"

	attachmentpb "github.com/hellopoisonx/aim/app/attachment/rpc/pb"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type InitAttachmentUploadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewInitAttachmentUploadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *InitAttachmentUploadLogic {
	return &InitAttachmentUploadLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *InitAttachmentUploadLogic) InitAttachmentUpload(req *types.InitAttachmentUploadRequest) (resp *types.InitAttachmentUploadResponse, err error) {
	userID, err := currentUserID(l.ctx)
	if err != nil {
		return nil, err
	}
	if l.svcCtx.AttachmentClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "attachment service unavailable")
	}

	rpcResp, err := l.svcCtx.AttachmentClient.InitUpload(l.ctx, &attachmentpb.InitUploadReq{
		OwnerId:        userID,
		ConversationId: req.ConversationId,
		Kind:           req.Kind,
		OriginalName:   req.OriginalName,
		Mime:           req.Mime,
		Size:           req.Size,
		Sha256:         req.Sha256,
	})
	if err != nil {
		return nil, err
	}

	return &types.InitAttachmentUploadResponse{
		FileId:       rpcResp.GetFileId(),
		Bucket:       rpcResp.GetBucket(),
		ObjectKey:    rpcResp.GetObjectKey(),
		UploadUrl:    rpcResp.GetUploadUrl(),
		UploadMethod: rpcResp.GetUploadMethod(),
		Headers:      rpcResp.GetHeaders(),
		ExpiresAt:    rpcResp.GetExpiresAt(),
	}, nil
}
