package bot

import (
	"context"
	"slices"

	attachmentpb "github.com/hellopoisonx/aim/app/attachment/rpc/pb"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/conversationservice"
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

	if l.svcCtx.AttachmentClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	// 1. Get file metadata to learn the conversation_id.
	fileInfo, err := l.svcCtx.AttachmentClient.GetFile(l.ctx, &attachmentpb.GetFileReq{
		UserId: identity.BotUserID,
		FileId: fileID,
	})
	if err != nil {
		if ce := errorx.FromGRPCError(err); ce != nil {
			return nil, ce
		}
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	if fileInfo.GetStatus() != "uploaded" {
		return nil, errorx.NewCodeError(errorx.CodeNotFound, "attachment not available")
	}

	conversationID := fileInfo.GetConversationId()
	if conversationID <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeNotFound, "attachment has no associated conversation")
	}

	// 2. Verify the bot is a member of the conversation that owns the attachment.
	if l.svcCtx.LogicConversationClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	membersResp, err := l.svcCtx.LogicConversationClient.GetConversationMembers(l.ctx,
		&conversationservice.GetConversationMembersReq{
			ConversationId: conversationID,
		})
	if err != nil {
		if ce := errorx.FromGRPCError(err); ce != nil {
			return nil, ce
		}
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	if !slices.Contains(membersResp.GetMemberIds(), identity.BotUserID) {
		return nil, errorx.NewCodeError(errorx.CodeForbidden, "bot is not a member of this conversation")
	}

	// 3. Authorize a presigned download URL.
	downloadResp, err := l.svcCtx.AttachmentClient.AuthorizeDownload(l.ctx, &attachmentpb.AuthorizeDownloadReq{
		UserId: identity.BotUserID,
		FileId: fileID,
	})
	if err != nil {
		if ce := errorx.FromGRPCError(err); ce != nil {
			return nil, ce
		}
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	return &types.BotDownloadAttachmentResponse{
		Url:       downloadResp.GetUrl(),
		Headers:   downloadResp.GetHeaders(),
		ExpiresAt: downloadResp.GetExpiresAt(),
	}, nil
}
