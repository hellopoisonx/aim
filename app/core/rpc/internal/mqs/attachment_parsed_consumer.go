package mqs

import (
	"context"
	"encoding/json"
	"fmt"

	attachmentpb "github.com/hellopoisonx/aim/app/attachment/rpc/pb"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	logicpb "github.com/hellopoisonx/aim/app/logic/rpc/pb"
	sharedattachment "github.com/hellopoisonx/aim/app/shared/attachment"
	"github.com/hellopoisonx/aim/app/shared/tracing"
	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

// AttachmentParsedConsumer 消费 aim.attachment.parsed 事件，
// 将附件解析结果（缩略图、尺寸、时长等）以系统消息形式推送至会话所有成员。
type AttachmentParsedConsumer struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAttachmentParsedConsumer(ctx context.Context, svcCtx *svc.ServiceContext) *AttachmentParsedConsumer {
	return &AttachmentParsedConsumer{ctx: ctx, svcCtx: svcCtx}
}

// Consume 实现 kq.ConsumeHandler 接口。
func (c *AttachmentParsedConsumer) Consume(ctx context.Context, key string, value string) error {
	var event struct {
		tracing.TraceContextFields
		FileID             string         `json:"file_id"`
		Kind               string         `json:"kind"`
		ParseStatus        string         `json:"parse_status"`
		ThumbnailObjectKey string         `json:"thumbnail_object_key,omitempty"`
		ThumbnailFileID    string         `json:"thumbnail_file_id,omitempty"`
		DurationMS         int64          `json:"duration_ms,omitempty"`
		Width              int            `json:"width,omitempty"`
		Height             int            `json:"height,omitempty"`
		Metadata           map[string]any `json:"metadata,omitempty"`
		Error              string         `json:"error,omitempty"`
		ParsedAt           int64          `json:"parsed_at"`
	}

	if err := json.Unmarshal([]byte(value), &event); err != nil {
		logx.WithContext(ctx).Errorf("failed to unmarshal attachment parsed event: %v", err)
		return err
	}

	ctx = tracing.ExtractTraceContext(ctx, event.TraceContextFields)

	ctx, span := tracing.StartKafkaConsumerSpan(ctx, "core.kafka.attachment_parsed.consume")
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
	metaBytes, _ := json.Marshal(event.Metadata)
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
	targetUserIDs, conversationType, err := resolveConversationMembers(ctx, c.svcCtx, conversationID)
	if err != nil {
		span.RecordError(err)
		return err
	}

	// 生成系统消息 ID。
	msgID, err := c.svcCtx.Snowflake.NextID()
	if err != nil {
		span.RecordError(err)
		return err
	}

	// 推送到每个会话成员所在网关节点。
	for _, targetUserID := range targetUserIDs {
		nodeIDs := userGatewayNodesForConsumer(ctx, c.svcCtx, targetUserID)
		if len(nodeIDs) == 0 {
			logx.WithContext(ctx).Debugf("no gateway nodes for user %d, skipping attachment parsed push", targetUserID)
			continue
		}

		for _, nodeID := range nodeIDs {
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

			resp, pushErr := pushMessageToNode(ctx, c.svcCtx.GatewayClient, nodeID, req)
			if pushErr != nil {
				span.RecordError(pushErr)
				logx.WithContext(ctx).Errorf("failed to push attachment parsed to user %d on node %s: %v", targetUserID, nodeID, pushErr)
				return pushErr
			}

			logx.WithContext(ctx).Debugf("attachment parsed pushed to user %d on node %s, success=%v", targetUserID, nodeID, resp.Success)
		}
	}

	return nil
}

// userGatewayNodesForConsumer 查询用户当前连接的网关节点 ID 列表。
// 优先使用 PresenceStore（L1 Cached + Redis Set），回退到 Redis SMembers，再回退空节点（单 gateway 模式）。
func userGatewayNodesForConsumer(ctx context.Context, svcCtx *svc.ServiceContext, userID int64) []string {
	if svcCtx.PresenceStore != nil {
		nodes, err := svcCtx.PresenceStore.GetUserGatewayNodes(ctx, userID)
		if err == nil && len(nodes) > 0 {
			return nodes
		}
	}

	if svcCtx.RedisClient != nil {
		key := fmt.Sprintf("%s%d", userGatewayKeyPrefix, userID)
		nodes, err := svcCtx.RedisClient.SMembers(ctx, key).Result()
		if err == nil && len(nodes) > 0 {
			return nodes
		}
	}

	return []string{""}
}

// resolveConversationMembers 查询会话成员列表及会话类型。
func resolveConversationMembers(ctx context.Context, svcCtx *svc.ServiceContext, conversationID int64) ([]int64, string, error) {
	if svcCtx.LogicConversationClient == nil {
		return nil, "", fmt.Errorf("logic conversation client not configured")
	}

	resp, err := svcCtx.LogicConversationClient.GetConversationMembers(ctx, &logicpb.GetConversationMembersReq{
		ConversationId: conversationID,
	})
	if err != nil {
		return nil, "", err
	}

	memberIDs := resp.GetMemberIds()
	if len(memberIDs) == 0 {
		return nil, resp.GetConversationType(), nil
	}

	seen := make(map[int64]struct{}, len(memberIDs))
	targets := make([]int64, 0, len(memberIDs))
	for _, memberID := range memberIDs {
		if memberID <= 0 {
			continue
		}
		if _, ok := seen[memberID]; ok {
			continue
		}
		seen[memberID] = struct{}{}
		targets = append(targets, memberID)
	}

	return targets, resp.GetConversationType(), nil
}
