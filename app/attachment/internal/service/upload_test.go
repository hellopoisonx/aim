package service

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hellopoisonx/aim/app/attachment/internal/config"
	sharedattachment "github.com/hellopoisonx/aim/app/shared/attachment"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/s3signer"
	"github.com/stretchr/testify/require"
)

// newTestService builds a Service without a database: the validation and
// presigning paths do not touch persistence, and GetFile reports ErrNotFound
// when no store is configured.
func newTestService() *Service {
	cfg := config.Config{}
	cfg.Seaweed.Endpoint = "http://seaweed-s3:8333"
	cfg.Seaweed.PublicEndpoint = "http://localhost:8333"
	cfg.Seaweed.Bucket = "aim-attachments"
	cfg.Seaweed.Region = "us-east-1"

	return &Service{
		cfg: cfg,
		signer: s3signer.S3Signer{
			Endpoint:       cfg.Seaweed.Endpoint,
			PublicEndpoint: cfg.Seaweed.PublicEndpoint,
			Bucket:         cfg.Seaweed.Bucket,
			Region:         cfg.Seaweed.Region,
		},
	}
}

func requireCode(t *testing.T, err error, want int) {
	t.Helper()

	var codeErr *errorx.CodeError
	require.ErrorAs(t, err, &codeErr)
	require.Equal(t, want, codeErr.Code)
}

func TestInitUploadValidatesRequest(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	_, err := svc.InitUpload(ctx, InitUploadRequest{Kind: sharedattachment.KindImage, Mime: "image/png", Size: 10})
	requireCode(t, err, errorx.CodeBadInput)

	_, err = svc.InitUpload(ctx, InitUploadRequest{OwnerID: 1, ConversationID: 1, Kind: "bogus", Mime: "image/png", Size: 10})
	requireCode(t, err, errorx.CodeBadInput)

	_, err = svc.InitUpload(ctx, InitUploadRequest{OwnerID: 1, ConversationID: 1, Kind: sharedattachment.KindImage, Mime: "image/png", Size: 0})
	requireCode(t, err, errorx.CodeBadInput)

	// A generic file must not accept an image mime.
	_, err = svc.InitUpload(ctx, InitUploadRequest{OwnerID: 1, ConversationID: 1, Kind: sharedattachment.KindFile, Mime: "image/png", Size: 10})
	requireCode(t, err, errorx.CodeBadInput)
}

func TestInitUploadPresignsPutURL(t *testing.T) {
	svc := newTestService()

	resp, err := svc.InitUpload(context.Background(), InitUploadRequest{
		OwnerID:        1,
		ConversationID: 2,
		Kind:           sharedattachment.KindImage,
		Mime:           "image/png",
		Size:           1024,
		OriginalName:   "cat.png",
	})
	require.NoError(t, err)

	require.NotEmpty(t, resp.FileID)
	require.Equal(t, "aim-attachments", resp.Bucket)
	require.Equal(t, "PUT", resp.UploadMethod)
	require.Equal(t, "image/png", resp.Headers["content-type"])
	require.Greater(t, resp.ExpiresAt, time.Now().UnixMilli())

	parsed, err := url.Parse(resp.UploadURL)
	require.NoError(t, err)
	require.Equal(t, "localhost:8333", parsed.Host, "clients upload through the public endpoint")
	require.Contains(t, parsed.Path, resp.ObjectKey)
	// Without S3 credentials (local SeaweedFS) the URL is issued unsigned.
	require.Empty(t, parsed.Query().Get("X-Amz-Signature"))

	// The object key is scoped by conversation and file id.
	require.Contains(t, resp.ObjectKey, "/2/"+resp.FileID+"/original")
}

func TestDownloadWithoutStoreReportsNotFound(t *testing.T) {
	svc := newTestService()

	_, err := svc.Download(context.Background(), 1, uuid.NewString())
	require.ErrorIs(t, err, ErrNotFound)

	_, err = svc.Download(context.Background(), 1, "not-a-uuid")
	require.Error(t, err)
}

func TestValidateReferenceRejectsUnknownFile(t *testing.T) {
	svc := newTestService()

	_, err := svc.ValidateReference(context.Background(), 1, 2, uuid.NewString(), sharedattachment.KindImage)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestAllowedMimeMatchesKind(t *testing.T) {
	require.True(t, allowedMime(sharedattachment.KindImage, "image/png"))
	require.True(t, allowedMime(sharedattachment.KindVideo, "video/mp4"))
	require.True(t, allowedMime(sharedattachment.KindAudio, "audio/mpeg"))
	require.False(t, allowedMime(sharedattachment.KindImage, "application/pdf"))
	require.False(t, allowedMime(sharedattachment.KindFile, "image/png"))
}

func TestInitUploadSignsURLWhenCredentialsConfigured(t *testing.T) {
	svc := newTestService()
	svc.signer.AccessKey = "access"
	svc.signer.SecretKey = "secret"

	resp, err := svc.InitUpload(context.Background(), InitUploadRequest{
		OwnerID:        1,
		ConversationID: 2,
		Kind:           sharedattachment.KindImage,
		Mime:           "image/png",
		Size:           1024,
	})
	require.NoError(t, err)

	parsed, err := url.Parse(resp.UploadURL)
	require.NoError(t, err)
	require.NotEmpty(t, parsed.Query().Get("X-Amz-Signature"))
	require.Equal(t, "content-type;host", parsed.Query().Get("X-Amz-SignedHeaders"))
}
