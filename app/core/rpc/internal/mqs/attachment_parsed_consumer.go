package mqs

import (
	"context"
	"encoding/json"

	attachmentpb "github.com/hellopoisonx/aim/app/attachment/rpc/pb"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	sharedattachment "github.com/hellopoisonx/aim/app/shared/attachment"
	"github.com/hellopoisonx/aim/app/shared/events"
	"github.com/hellopoisonx/aim/app/shared/tracing"
	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

// AttachmentParsedConsumer 消费 aim-attachment-parsed 事件，
// 将附件解析结果（缩略图、尺寸、时长等）以系统消息形式推送至会话所有成员。
type AttachmentParsedConsumer struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAttachmentParsedConsumer(ctx context.Context, svcCtx *svc.ServiceContext) *AttachmentParsedConsumer {
	return &AttachmentParsedConsumer{ctx: ctx, svcCtx: svcCtx}
}

// Consume 实现 rocketmq 消息处理接口。
func (c *AttachmentParsedConsumer) Consume(ctx context.Context, key string, value string) error {
	event, metaBytes, err := decodeAttachmentParsedEvent([]byte(value))
	if err != nil {
		logx.WithContext(ctx).Errorf(
			"dropping undecodable attachment parsed event key=%s: %v payload=%q", key, err, value,
		)
		return nil
	}

	ctx = tracing.ExtractTraceContext(ctx, event.TraceContextFields)

	ctx, span := tracing.StartMQConsumerSpan(ctx, "core.rocketmq.attachment_parsed.consume")
	defer span.End()

	// 仅处理解析成功的事件；失败事件暂不推送，客户端可通过轮询 GetFile 感知。
	if event.ParseStatus != sharedattachment.ParseStatusReady {
		logx.WithContext(ctx).Infof("attachment parsed event skipped: parse_status=%s file_id=%s", event.ParseStatus, event.FileID)
		return nil
	}

	logx.WithContext(ctx).Infof("attachment parsed: file_id=%s kind=%s width=%d height=%d duration_ms=%d",
		event.FileID, event.Kind, event.Width, event.Height, event.DurationMS)

	// 从 attachment 服务获取完整文件信息（含 conversation_id、原始名称等）。
	// user_id=0 利用 GetFile 的 OR f.status='uploaded' 条件访问已上传文件。
	fileInfo, err := c.svcCtx.AttachmentClient.GetFile(ctx, &attachmentpb.GetFileReq{
		UserId: 0,
		FileId: event.FileID,
	})
	if err != nil {
		span.RecordError(err)
		logx.WithContext(ctx).Errorf("failed to get file info for %s: %v", event.FileID, err)
		return err
	}

	conversationID := fileInfo.GetConversationId()
	if conversationID <= 0 {
		logx.WithContext(ctx).Errorf("attachment %s has invalid conversation_id %d", event.FileID, conversationID)
		return nil
	}

	// 构建更新后的附件 Content JSON。
	content := sharedattachment.Content{
		Schema: sharedattachment.ContentSchemaV1,
		FileID: event.FileID,
		Kind:   event.Kind,
		Original: sharedattachment.OriginalObject{
			Name:   fileInfo.GetOriginalName(),
			Mime:   fileInfo.GetMime(),
			Size:   fileInfo.GetSize(),
			SHA256: fileInfo.GetSha256(),
		},
		ThumbnailFileID: event.ThumbnailObjectKey,
		ParseStatus:     event.ParseStatus,
		DurationMS:      event.DurationMS,
		Width:           event.Width,
		Height:          event.Height,
		Metadata:        metaBytes,
	}
	contentStr, err := content.Marshal()
	if err != nil {
		span.RecordError(err)
		logx.WithContext(ctx).Errorf("failed to marshal attachment content for %s: %v", event.FileID, err)
		return err
	}

	// 获取会话成员列表。
	targetUserIDs, conversationType, err := conversationRecipients(ctx, c.svcCtx, conversationID)
	if err != nil {
		span.RecordError(err)
		return err
	}
	if len(targetUserIDs) == 0 {
		logx.WithContext(ctx).Infof("attachment parsed event for %s has no conversation members, skipping push", event.FileID)
		return nil
	}

	// 生成系统消息 ID。
	msgID, err := c.svcCtx.Snowflake.NextID()
	if err != nil {
		span.RecordError(err)
		return err
	}

	// 推送到每个会话成员所在网关节点。
	pushErr := pushToMemberNodes(ctx, c.svcCtx, targetUserIDs, 0, "", func(targetUserID int64, nodeID string) error {
		req := &gwpb.PushMessageReq{
			MessageId:        msgID,
			ConversationId:   conversationID,
			ConversationType: conversationType,
			MessageType:      event.Kind,
			Content:          contentStr,
			SenderId:         0, // 系统消息
			SentAt:           event.ParsedAt,
			TargetUserId:     targetUserID,
			IsSystem:         true,
			SenderInfo:       gatewaySystemSenderInfo(),
		}

		resp, err := c.svcCtx.GatewayClient.PushMessageToNode(ctx, nodeID, req)
		if err != nil {
			return err
		}

		logx.WithContext(ctx).Debugf("attachment parsed pushed to user %d on node %s, success=%v", targetUserID, nodeID, resp.Success)

		return nil
	})
	if pushErr != nil {
		span.RecordError(pushErr)
		return pushErr
	}

	return nil
}

// decodeAttachmentParsedEvent decodes the shared event and returns the metadata
// exactly as published: it is echoed into message content verbatim, so it is
// captured raw rather than re-marshalled through MediaMetadata.
func decodeAttachmentParsedEvent(value []byte) (events.AttachmentParsedEvent, json.RawMessage, error) {
	var wire struct {
		events.AttachmentParsedEvent
		Metadata json.RawMessage `json:"metadata,omitempty"`
	}

	if err := json.Unmarshal(value, &wire); err != nil {
		return events.AttachmentParsedEvent{}, nil, err
	}

	return wire.AttachmentParsedEvent, wire.Metadata, nil
}
