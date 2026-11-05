package service

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"

	sharedattachment "github.com/hellopoisonx/aim/app/shared/attachment"
)

// Content helpers shared with message rendering.

func allowedMime(kind, mime string) bool {
	mime = strings.TrimSpace(strings.ToLower(mime))
	if mime == "" || !strings.Contains(mime, "/") {
		return false
	}
	switch kind {
	case sharedattachment.KindImage:
		return strings.HasPrefix(mime, "image/")
	case sharedattachment.KindVideo:
		return strings.HasPrefix(mime, "video/")
	case sharedattachment.KindAudio:
		return strings.HasPrefix(mime, "audio/")
	case sharedattachment.KindFile:
		return !strings.HasPrefix(mime, "image/") && !strings.HasPrefix(mime, "video/") && !strings.HasPrefix(mime, "audio/")
	default:
		return false
	}
}

func ContentFromFile(info *FileInfo) (string, error) {
	name := info.OriginalName
	if name == "" {
		name = filepath.Base(info.ObjectKey)
	}
	meta, _ := json.Marshal(info.Metadata)
	c := sharedattachment.Content{Schema: sharedattachment.ContentSchemaV1, FileID: info.FileID, Kind: info.Kind, Original: sharedattachment.OriginalObject{Name: name, Mime: info.Mime, Size: info.Size, SHA256: info.SHA256}, ThumbnailFileID: info.ThumbnailObjectKey, ParseStatus: info.ParseStatus, DurationMS: info.DurationMS, Width: info.Width, Height: info.Height, Metadata: bytes.TrimSpace(meta)}
	return c.Marshal()
}
