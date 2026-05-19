package mqs

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/shared/events"
	"github.com/hellopoisonx/aim/app/shared/tracing"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"

	"github.com/zeromicro/go-zero/core/logx"
)

type UserCreatedConsumer struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUserCreatedConsumer(ctx context.Context, svcCtx *svc.ServiceContext) *UserCreatedConsumer {
	return &UserCreatedConsumer{ctx: ctx, svcCtx: svcCtx}
}

// Consume implements the kq.ConsumeHandler interface.
func (c *UserCreatedConsumer) Consume(ctx context.Context, key string, value string) error {
	var event events.UserCreatedEvent
	if err := json.Unmarshal([]byte(value), &event); err != nil {
		logx.WithContext(ctx).Errorf("failed to unmarshal user created event: %v", err)
		return err
	}

	ctx = tracing.ExtractTraceContext(ctx, event.TraceContextFields)

	ctx, span := tracing.StartKafkaConsumerSpan(ctx, "logic.kafka.user_created.consume")
	defer span.End()

	// Skip if UserInfoService is not available (DB disabled)
	if c.svcCtx.UserInfoService == nil {
		err := errors.New("user info service is not configured")
		span.RecordError(err)
		logx.WithContext(ctx).Errorf("failed to consume user created event for %d: %v", event.UserID, err)

		return err
	}

	_, err := c.svcCtx.UserInfoService.CreateUserInfo(ctx, event.UserID, event.Email, event.Nickname, event.Avatar)
	if err != nil {
		if errors.Is(err, service.ErrUserExists) {
			logx.WithContext(ctx).Infof("user %d already exists, skipping (idempotent)", event.UserID)
			return nil
		}

		logx.WithContext(ctx).Errorf("failed to create user info for %d: %v", event.UserID, err)

		return err
	}

	logx.WithContext(ctx).Infof("created user info for %d", event.UserID)

	return nil
}
