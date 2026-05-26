package server

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	attachmentservice "github.com/hellopoisonx/aim/app/attachment/internal/service"
	"github.com/hellopoisonx/aim/app/attachment/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/jackc/pgx/v5"
)

type AttachmentServiceServer struct {
	pb.UnimplementedAttachmentServiceServer
	svc *attachmentservice.Service
}

func NewAttachmentServiceServer(svc *attachmentservice.Service) *AttachmentServiceServer {
	return &AttachmentServiceServer{svc: svc}
}

func (s *AttachmentServiceServer) InitUpload(ctx context.Context, req *pb.InitUploadReq) (*pb.InitUploadResp, error) {
	resp, err := s.svc.InitUpload(ctx, attachmentservice.InitUploadRequest{
		OwnerID:        req.GetOwnerId(),
		ConversationID: req.GetConversationId(),
		Kind:           req.GetKind(),
		OriginalName:   req.GetOriginalName(),
		Mime:           req.GetMime(),
		Size:           req.GetSize(),
		SHA256:         req.GetSha256(),
	})
	if err != nil {
		return nil, toRPCError(err)
	}
	return &pb.InitUploadResp{
		FileId:       resp.FileID,
		Bucket:       resp.Bucket,
		ObjectKey:    resp.ObjectKey,
		UploadUrl:    resp.UploadURL,
		UploadMethod: resp.UploadMethod,
		Headers:      resp.Headers,
		ExpiresAt:    resp.ExpiresAt,
	}, nil
}

func (s *AttachmentServiceServer) CompleteUpload(ctx context.Context, req *pb.CompleteUploadReq) (*pb.FileInfo, error) {
	info, err := s.svc.CompleteUpload(ctx, req.GetUserId(), req.GetFileId(), attachmentservice.CompleteUploadRequest{SHA256: req.GetSha256()})
	if err != nil {
		return nil, toRPCError(err)
	}
	return fileInfoToPB(info), nil
}

func (s *AttachmentServiceServer) GetFile(ctx context.Context, req *pb.GetFileReq) (*pb.FileInfo, error) {
	info, err := s.svc.GetFile(ctx, req.GetUserId(), req.GetFileId())
	if err != nil {
		return nil, toRPCError(err)
	}
	return fileInfoToPB(info), nil
}

func (s *AttachmentServiceServer) AuthorizeDownload(ctx context.Context, req *pb.AuthorizeDownloadReq) (*pb.AuthorizeDownloadResp, error) {
	resp, err := s.svc.Download(ctx, req.GetUserId(), req.GetFileId())
	if err != nil {
		return nil, toRPCError(err)
	}
	return &pb.AuthorizeDownloadResp{Url: resp.URL, Headers: resp.Headers, ExpiresAt: resp.ExpiresAt}, nil
}

func (s *AttachmentServiceServer) ValidateReference(ctx context.Context, req *pb.ValidateReferenceReq) (*pb.FileInfo, error) {
	info, err := s.svc.ValidateReference(ctx, req.GetUserId(), req.GetConversationId(), req.GetFileId(), req.GetKind())
	if err != nil {
		return nil, toRPCError(err)
	}
	return fileInfoToPB(info), nil
}

func fileInfoToPB(info *attachmentservice.FileInfo) *pb.FileInfo {
	if info == nil {
		return nil
	}
	metadataJSON := "{}"
	if info.Metadata != nil {
		if b, err := json.Marshal(info.Metadata); err == nil {
			metadataJSON = string(b)
		}
	}
	return &pb.FileInfo{
		FileId:             info.FileID,
		OwnerId:            info.OwnerID,
		ConversationId:     info.ConversationID,
		Kind:               info.Kind,
		OriginalName:       info.OriginalName,
		Mime:               info.Mime,
		Size:               info.Size,
		Sha256:             info.SHA256,
		Status:             info.Status,
		ParseStatus:        info.ParseStatus,
		Bucket:             info.Bucket,
		ObjectKey:          info.ObjectKey,
		ThumbnailObjectKey: info.ThumbnailObjectKey,
		DurationMs:         info.DurationMS,
		Width:              safeInt32(info.Width),
		Height:             safeInt32(info.Height),
		MetadataJson:       metadataJSON,
	}
}

func toRPCError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, attachmentservice.ErrNotFound) || errors.Is(err, pgx.ErrNoRows) {
		return errorx.NewCodeError(errorx.CodeNotFound, "attachment not found")
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "denied"):
		return errorx.NewCodeError(errorx.CodeForbidden, msg)
	case strings.Contains(lower, "required"),
		strings.Contains(lower, "unsupported"),
		strings.Contains(lower, "not allowed"),
		strings.Contains(lower, "invalid"),
		strings.Contains(lower, "must be"),
		strings.Contains(lower, "not uploaded"):
		return errorx.NewCodeError(errorx.CodeBadInput, msg)
	default:
		return errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}
}

func safeInt32(v int) int32 {
	const (
		maxInt32 = int64(1<<31 - 1)
		minInt32 = int64(-1 << 31)
	)
	iv := int64(v)
	if iv > maxInt32 {
		return 1<<31 - 1
	}
	if iv < minInt32 {
		return -1 << 31
	}
	// #nosec G115 -- value is clamped to int32 range above.
	return int32(v)
}
