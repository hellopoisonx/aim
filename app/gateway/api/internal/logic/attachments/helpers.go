package attachments

import (
	"encoding/json"
	"strings"

	attachmentpb "github.com/hellopoisonx/aim/app/attachment/rpc/pb"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
)

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
