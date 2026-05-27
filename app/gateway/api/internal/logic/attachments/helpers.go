package attachments

import (
	"context"
	"encoding/json"
	"strings"

	attachmentpb "github.com/hellopoisonx/aim/app/attachment/rpc/pb"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	"github.com/hellopoisonx/aim/app/shared/errorx"
)

func currentUserID(ctx context.Context) (int64, error) {
	identity, ok := ws.IdentityFromContext(ctx)
	if !ok || identity.UserID <= 0 {
		return 0, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}

	return identity.UserID, nil
}

func fileInfoFromPB(info *attachmentpb.FileInfo) *types.AttachmentFileInfo {
	if info == nil {
		return nil
	}

	metadata := map[string]any{}
	if raw := strings.TrimSpace(info.GetMetadataJson()); raw != "" {
		_ = json.Unmarshal([]byte(raw), &metadata)
	}

	return &types.AttachmentFileInfo{
		FileId:             info.GetFileId(),
		OwnerId:            info.GetOwnerId(),
		ConversationId:     info.GetConversationId(),
		Kind:               info.GetKind(),
		OriginalName:       info.GetOriginalName(),
		Mime:               info.GetMime(),
		Size:               info.GetSize(),
		Sha256:             info.GetSha256(),
		Status:             info.GetStatus(),
		ParseStatus:        info.GetParseStatus(),
		Bucket:             info.GetBucket(),
		ObjectKey:          info.GetObjectKey(),
		ThumbnailObjectKey: info.GetThumbnailObjectKey(),
		DurationMs:         info.GetDurationMs(),
		Width:              info.GetWidth(),
		Height:             info.GetHeight(),
		Metadata:           metadata,
	}
}
