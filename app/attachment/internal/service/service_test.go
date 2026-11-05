package service

import (
	"testing"

	"github.com/google/uuid"
	sharedattachment "github.com/hellopoisonx/aim/app/shared/attachment"
	"github.com/hellopoisonx/aim/app/shared/attachmentstore/dao"
	"github.com/hellopoisonx/aim/app/shared/pgtypes"
	"github.com/stretchr/testify/require"
)

func TestFileInfoMapsRow(t *testing.T) {
	id := uuid.New()
	info := fileInfo(dao.FileRow{
		FileID:             id,
		OwnerID:            7,
		ConversationID:     3,
		Kind:               sharedattachment.KindImage,
		OriginalName:       "cat.png",
		Mime:               "image/png",
		SizeBytes:          42,
		Sha256:             "abc",
		Status:             sharedattachment.UploadStatusUploaded,
		ParseStatus:        sharedattachment.ParseStatusReady,
		Bucket:             "aim",
		ObjectKey:          "k/original",
		ThumbnailObjectKey: "k/thumbnail",
		DurationMS:         1500,
		Width:              640,
		Height:             480,
		Metadata:           pgtypes.JSON(`{"pages":2}`),
	})

	require.Equal(t, id.String(), info.FileID)
	require.Equal(t, int64(42), info.Size)
	require.Equal(t, 640, info.Width)
	require.Equal(t, 480, info.Height)
	require.Equal(t, "k/thumbnail", info.ThumbnailObjectKey)
	require.Equal(t, int64(1500), info.DurationMS)
	require.Equal(t, map[string]any{"pages": float64(2)}, info.Metadata)
}

func TestAllowedMimeForGenericFile(t *testing.T) {
	require.True(t, allowedMime(sharedattachment.KindFile, "application/pdf"))
	require.True(t, allowedMime(sharedattachment.KindFile, "application/octet-stream"))
	require.True(t, allowedMime(sharedattachment.KindFile, "text/plain"))
	require.False(t, allowedMime(sharedattachment.KindFile, "image/png"))
	require.False(t, allowedMime(sharedattachment.KindFile, "video/mp4"))
	require.False(t, allowedMime(sharedattachment.KindFile, "audio/mpeg"))
	require.False(t, allowedMime(sharedattachment.KindFile, ""))
	require.False(t, allowedMime(sharedattachment.KindFile, "pdf"))
}

func TestAllowedMimeForMedia(t *testing.T) {
	require.True(t, allowedMime(sharedattachment.KindImage, "IMAGE/PNG"))
	require.True(t, allowedMime(sharedattachment.KindVideo, "video/mp4"))
	require.True(t, allowedMime(sharedattachment.KindAudio, "audio/ogg"))
	require.False(t, allowedMime(sharedattachment.KindImage, "application/pdf"))
	require.False(t, allowedMime("unknown", "application/pdf"))
}
