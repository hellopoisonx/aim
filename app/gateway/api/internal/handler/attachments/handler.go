package attachments

import (
	"encoding/json"
	"net/http"
	"strings"

	attachmentpb "github.com/hellopoisonx/aim/app/attachment/rpc/pb"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/zeromicro/go-zero/rest/httpx"
)

type initAttachmentUploadRequest struct {
	ConversationId int64  `json:"conversation_id" validate:"required"`
	Kind           string `json:"kind" validate:"required,oneof=image video audio"`
	OriginalName   string `json:"original_name" validate:"required"`
	Mime           string `json:"mime" validate:"required"`
	Size           int64  `json:"size" validate:"required"`
	Sha256         string `json:"sha256,omitempty"`
}

type initAttachmentUploadResponse struct {
	FileId       string            `json:"file_id"`
	Bucket       string            `json:"bucket"`
	ObjectKey    string            `json:"object_key"`
	UploadUrl    string            `json:"upload_url"`
	UploadMethod string            `json:"upload_method"`
	Headers      map[string]string `json:"headers,omitempty"`
	ExpiresAt    int64             `json:"expires_at"`
}

type completeAttachmentUploadRequest struct {
	Id     string `path:"id" validate:"required"`
	Sha256 string `json:"sha256,omitempty"`
}

type getAttachmentRequest struct {
	Id string `path:"id" validate:"required"`
}

type downloadAttachmentRequest struct {
	Id string `path:"id" validate:"required"`
}

type downloadAttachmentResponse struct {
	Url       string            `json:"url"`
	Headers   map[string]string `json:"headers,omitempty"`
	ExpiresAt int64             `json:"expires_at"`
}

type attachmentFileInfo struct {
	FileId             string         `json:"file_id"`
	OwnerId            int64          `json:"owner_id"`
	ConversationId     int64          `json:"conversation_id"`
	Kind               string         `json:"kind"`
	OriginalName       string         `json:"original_name"`
	Mime               string         `json:"mime"`
	Size               int64          `json:"size"`
	Sha256             string         `json:"sha256,omitempty"`
	Status             string         `json:"status"`
	ParseStatus        string         `json:"parse_status"`
	Bucket             string         `json:"bucket"`
	ObjectKey          string         `json:"object_key"`
	ThumbnailObjectKey string         `json:"thumbnail_object_key,omitempty"`
	DurationMs         int64          `json:"duration_ms,omitempty"`
	Width              int32          `json:"width,omitempty"`
	Height             int32          `json:"height,omitempty"`
	Metadata           map[string]any `json:"metadata,omitempty"`
}

func Handler(serverCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := ws.IdentityFromContext(r.Context())
		if !ok || identity.UserID <= 0 {
			httpx.ErrorCtx(r.Context(), w, errorx.NewCodeError(errorx.CodeAuth, "unauthorized"))
			return
		}
		if serverCtx.AttachmentClient == nil {
			httpx.ErrorCtx(r.Context(), w, errorx.NewCodeError(errorx.CodeInternal, "attachment service unavailable"))
			return
		}

		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/init"):
			handleInitUpload(w, r, serverCtx, identity.UserID)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/complete"):
			handleCompleteUpload(w, r, serverCtx, identity.UserID)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/download"):
			handleDownload(w, r, serverCtx, identity.UserID)
		case r.Method == http.MethodGet:
			handleGetFile(w, r, serverCtx, identity.UserID)
		default:
			httpx.ErrorCtx(r.Context(), w, errorx.NewCodeError(errorx.CodeBadInput, "unsupported attachment route"))
		}
	}
}

func handleInitUpload(w http.ResponseWriter, r *http.Request, serverCtx *svc.ServiceContext, userID int64) {
	var req initAttachmentUploadRequest
	if err := httpx.Parse(r, &req); err != nil {
		httpx.ErrorCtx(r.Context(), w, err)
		return
	}
	resp, err := serverCtx.AttachmentClient.InitUpload(r.Context(), &attachmentpb.InitUploadReq{
		OwnerId:        userID,
		ConversationId: req.ConversationId,
		Kind:           req.Kind,
		OriginalName:   req.OriginalName,
		Mime:           req.Mime,
		Size:           req.Size,
		Sha256:         req.Sha256,
	})
	if err != nil {
		httpx.ErrorCtx(r.Context(), w, err)
		return
	}
	httpx.OkJsonCtx(r.Context(), w, &initAttachmentUploadResponse{
		FileId:       resp.GetFileId(),
		Bucket:       resp.GetBucket(),
		ObjectKey:    resp.GetObjectKey(),
		UploadUrl:    resp.GetUploadUrl(),
		UploadMethod: resp.GetUploadMethod(),
		Headers:      resp.GetHeaders(),
		ExpiresAt:    resp.GetExpiresAt(),
	})
}

func handleCompleteUpload(w http.ResponseWriter, r *http.Request, serverCtx *svc.ServiceContext, userID int64) {
	var req completeAttachmentUploadRequest
	if err := httpx.Parse(r, &req); err != nil {
		httpx.ErrorCtx(r.Context(), w, err)
		return
	}
	info, err := serverCtx.AttachmentClient.CompleteUpload(r.Context(), &attachmentpb.CompleteUploadReq{
		UserId: userID,
		FileId: req.Id,
		Sha256: req.Sha256,
	})
	if err != nil {
		httpx.ErrorCtx(r.Context(), w, err)
		return
	}
	httpx.OkJsonCtx(r.Context(), w, fileInfoFromPB(info))
}

func handleGetFile(w http.ResponseWriter, r *http.Request, serverCtx *svc.ServiceContext, userID int64) {
	var req getAttachmentRequest
	if err := httpx.Parse(r, &req); err != nil {
		httpx.ErrorCtx(r.Context(), w, err)
		return
	}
	info, err := serverCtx.AttachmentClient.GetFile(r.Context(), &attachmentpb.GetFileReq{UserId: userID, FileId: req.Id})
	if err != nil {
		httpx.ErrorCtx(r.Context(), w, err)
		return
	}
	httpx.OkJsonCtx(r.Context(), w, fileInfoFromPB(info))
}

func handleDownload(w http.ResponseWriter, r *http.Request, serverCtx *svc.ServiceContext, userID int64) {
	var req downloadAttachmentRequest
	if err := httpx.Parse(r, &req); err != nil {
		httpx.ErrorCtx(r.Context(), w, err)
		return
	}
	resp, err := serverCtx.AttachmentClient.AuthorizeDownload(r.Context(), &attachmentpb.AuthorizeDownloadReq{UserId: userID, FileId: req.Id})
	if err != nil {
		httpx.ErrorCtx(r.Context(), w, err)
		return
	}
	httpx.OkJsonCtx(r.Context(), w, &downloadAttachmentResponse{Url: resp.GetUrl(), Headers: resp.GetHeaders(), ExpiresAt: resp.GetExpiresAt()})
}

func fileInfoFromPB(info *attachmentpb.FileInfo) *attachmentFileInfo {
	if info == nil {
		return nil
	}
	metadata := map[string]any{}
	if raw := strings.TrimSpace(info.GetMetadataJson()); raw != "" {
		_ = json.Unmarshal([]byte(raw), &metadata)
	}
	return &attachmentFileInfo{
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
