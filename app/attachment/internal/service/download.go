package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	sharedattachment "github.com/hellopoisonx/aim/app/shared/attachment"
	"github.com/hellopoisonx/aim/app/shared/attachmentstore/dao"
)

// Read paths: file lookup, download presigning and reference validation.

func (s *Service) GetFile(ctx context.Context, userID int64, fileID string) (*FileInfo, error) {
	id, err := uuid.Parse(fileID)
	if err != nil {
		return nil, err
	}
	if s.store == nil {
		return nil, ErrNotFound
	}
	row, err := s.store.GetFile(ctx, id, userID)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	info := &FileInfo{
		FileID:             row.FileID.String(),
		OwnerID:            row.OwnerID,
		ConversationID:     row.ConversationID,
		Kind:               row.Kind,
		OriginalName:       row.OriginalName,
		Mime:               row.Mime,
		Size:               row.SizeBytes,
		SHA256:             row.Sha256,
		Status:             row.Status,
		ParseStatus:        row.ParseStatus,
		Bucket:             row.Bucket,
		ObjectKey:          row.ObjectKey,
		ThumbnailObjectKey: row.ThumbnailObjectKey,
		DurationMS:         row.DurationMS,
		Width:              int(row.Width),
		Height:             int(row.Height),
	}
	_ = json.Unmarshal(row.Metadata, &info.Metadata)
	return info, nil
}

func (s *Service) Download(ctx context.Context, userID int64, fileID string) (*DownloadResponse, error) {
	info, err := s.GetFile(ctx, userID, fileID)
	if err != nil {
		return nil, err
	}
	if info.Status != sharedattachment.UploadStatusUploaded {
		return nil, ErrNotUploaded
	}
	url, headers, err := s.signer.Presign(http.MethodGet, info.ObjectKey, s.cfg.Seaweed.GetDownloadTTL(), nil)
	if err != nil {
		return nil, err
	}
	return &DownloadResponse{URL: url, Headers: headers, ExpiresAt: time.Now().Add(s.cfg.Seaweed.GetDownloadTTL()).UnixMilli()}, nil
}

func (s *Service) ValidateReference(ctx context.Context, userID, conversationID int64, fileID, kind string) (*FileInfo, error) {
	info, err := s.GetFile(ctx, userID, fileID)
	if err != nil {
		return nil, err
	}
	if info.Status != sharedattachment.UploadStatusUploaded || info.OwnerID != userID || info.ConversationID != conversationID || info.Kind != kind {
		return nil, ErrReferenceDenied
	}
	return info, nil
}
