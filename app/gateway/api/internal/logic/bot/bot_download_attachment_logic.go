package bot

import (
	"context"

	attachmentpb "github.com/hellopoisonx/aim/app/attachment/rpc/pb"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	sharedattachment "github.com/hellopoisonx/aim/app/shared/attachment"
	"github.com/hellopoisonx/aim/app/shared/botperm"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

// botAttachmentDownloadAction is the token action required to download an attachment.
const botAttachmentDownloadAction = botperm.ActionAttachmentDownload

type BotDownloadAttachmentLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewBotDownloadAttachmentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BotDownloadAttachmentLogic {
	return &BotDownloadAttachmentLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *BotDownloadAttachmentLogic) BotDownloadAttachment(req *types.BotDownloadAttachmentRequest) (*types.BotDownloadAttachmentResponse, error) {
	identity, err := requireBotAction(l.ctx, botAttachmentDownloadAction)
	if err != nil {
		return nil, err
	}

	fileID := req.Id
	if fileID == "" {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "id is required")
	}

	// Get file metadata to learn the conversation_id.
	fileInfo, err := l.svcCtx.AttachmentClient.GetFile(l.ctx, &attachmentpb.GetFileReq{
		UserId: identity.BotUserID,
		FileId: fileID,
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "bot download attachment", err)
	}

	if fileInfo.GetStatus() != sharedattachment.UploadStatusUploaded {
		return nil, errorx.NewCodeError(errorx.CodeNotFound, "attachment not available")
	}

	conversationID := fileInfo.GetConversationId()
	if conversationID <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeNotFound, "attachment has no associated conversation")
	}

	// Verify the bot is a member of the conversation that owns the attachment.
	if err := ensureBotConversationMember(l.ctx, l.svcCtx.LogicConversationClient, identity, conversationID); err != nil {
		return nil, err
	}

	// Authorize a presigned download URL.
	downloadResp, err := l.svcCtx.AttachmentClient.AuthorizeDownload(l.ctx, &attachmentpb.AuthorizeDownloadReq{
		UserId: identity.BotUserID,
		FileId: fileID,
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "bot download attachment", err)
	}

	return &types.BotDownloadAttachmentResponse{
		Url:       downloadResp.GetUrl(),
		Headers:   downloadResp.GetHeaders(),
		ExpiresAt: downloadResp.GetExpiresAt(),
	}, nil
}
