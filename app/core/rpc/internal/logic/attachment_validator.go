package logic

import (
	"context"
	"fmt"

	attachmentpb "github.com/hellopoisonx/aim/app/attachment/rpc/pb"
	sharedattachment "github.com/hellopoisonx/aim/app/shared/attachment"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type attachmentReferenceValidator interface {
	ValidateReference(ctx context.Context, senderID, conversationID int64, kind, content string) error
}

type grpcAttachmentValidator struct {
	client attachmentpb.AttachmentServiceClient
}

func newGRPCAttachmentValidator(client attachmentpb.AttachmentServiceClient) attachmentReferenceValidator {
	if client == nil {
		return nil
	}
	return &grpcAttachmentValidator{client: client}
}

func (v *grpcAttachmentValidator) ValidateReference(ctx context.Context, senderID, conversationID int64, kind, rawContent string) error {
	content, err := sharedattachment.ParseContent(rawContent)
	if err != nil {
		return err
	}
	if content.Kind != kind {
		return fmt.Errorf("attachment kind does not match message_type")
	}

	ctx, span := otel.Tracer("github.com/hellopoisonx/aim/app/core/rpc").Start(
		ctx,
		"core.attachment.validate_reference",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("attachment.file_id", content.FileID)),
	)
	defer span.End()

	_, err = v.client.ValidateReference(ctx, &attachmentpb.ValidateReferenceReq{
		UserId:         senderID,
		ConversationId: conversationID,
		FileId:         content.FileID,
		Kind:           kind,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return nil
}
