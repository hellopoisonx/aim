package service

import (
	"testing"

	sharedattachment "github.com/hellopoisonx/aim/app/shared/attachment"
	"github.com/stretchr/testify/require"
)

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
