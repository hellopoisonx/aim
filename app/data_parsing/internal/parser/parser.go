package parser

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"strings"
)

const (
	thumbMIMEPNG              = "image/png"
	maxThumbDimension         = 512
	metadataParserPlaceholder = "placeholder"
)

type Result struct {
	Thumbnail   []byte
	ThumbMIME   string
	ThumbWidth  int
	ThumbHeight int
	Width       int
	Height      int
	DurationMS  int64
	Metadata    map[string]any
}

type Parser interface {
	Parse(ctx context.Context, kind, mime string, data []byte) (*Result, error)
}

type DefaultParser struct{}

func (DefaultParser) Parse(ctx context.Context, kind, mime string, data []byte) (*Result, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	res := &Result{Metadata: map[string]any{"mime": mime}}
	switch kind {
	case "image":
		thumb, thumbWidth, thumbHeight, width, height, format, err := imageThumbnail(data)
		if err != nil {
			res.Metadata["parser"] = metadataParserPlaceholder
			res.Metadata["thumbnail_fallback"] = err.Error()
			res.Thumbnail = transparentPNG()
			res.ThumbMIME = thumbMIMEPNG
			res.ThumbWidth = 1
			res.ThumbHeight = 1
			break
		}
		res.Width = width
		res.Height = height
		res.Metadata["format"] = format
		res.Thumbnail = thumb
		res.ThumbMIME = thumbMIMEPNG
		res.ThumbWidth = thumbWidth
		res.ThumbHeight = thumbHeight
	case "video":
		res.Metadata["parser"] = metadataParserPlaceholder
		res.Metadata["target_format"] = "h264/mp4+aac"
		res.Thumbnail = transparentPNG()
		res.ThumbMIME = thumbMIMEPNG
		res.ThumbWidth = 1
		res.ThumbHeight = 1
	case "audio":
		res.Metadata["parser"] = metadataParserPlaceholder
		res.Thumbnail = transparentPNG()
		res.ThumbMIME = thumbMIMEPNG
		res.ThumbWidth = 1
		res.ThumbHeight = 1
	default:
		return nil, fmt.Errorf("unsupported kind %q", kind)
	}
	if strings.HasPrefix(mime, "audio/") {
		res.Metadata["media_type"] = "audio"
	}
	return res, nil
}

func imageThumbnail(data []byte) ([]byte, int, int, int, int, string, error) {
	src, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, 0, 0, "", fmt.Errorf("decode image: %w", err)
	}
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil, 0, 0, 0, 0, "", fmt.Errorf("decode image: invalid dimensions %dx%d", width, height)
	}

	thumbWidth, thumbHeight := fitInside(width, height, maxThumbDimension)
	dst := image.NewRGBA(image.Rect(0, 0, thumbWidth, thumbHeight))
	resizeNearest(dst, src)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, 0, 0, 0, 0, "", fmt.Errorf("encode thumbnail: %w", err)
	}
	return buf.Bytes(), thumbWidth, thumbHeight, width, height, format, nil
}

func fitInside(width, height, maxDimension int) (int, int) {
	if maxDimension <= 0 {
		maxDimension = maxThumbDimension
	}
	if width <= maxDimension && height <= maxDimension {
		return width, height
	}
	if width >= height {
		thumbHeight := max(height*maxDimension/width, 1)
		return maxDimension, thumbHeight
	}
	thumbWidth := max(width*maxDimension/height, 1)
	return thumbWidth, maxDimension
}

func resizeNearest(dst *image.RGBA, src image.Image) {
	dstBounds := dst.Bounds()
	srcBounds := src.Bounds()
	dstWidth, dstHeight := dstBounds.Dx(), dstBounds.Dy()
	srcWidth, srcHeight := srcBounds.Dx(), srcBounds.Dy()
	for y := range dstHeight {
		sy := srcBounds.Min.Y + y*srcHeight/dstHeight
		for x := range dstWidth {
			sx := srcBounds.Min.X + x*srcWidth/dstWidth
			dst.Set(dstBounds.Min.X+x, dstBounds.Min.Y+y, src.At(sx, sy))
		}
	}
}

func transparentPNG() []byte {
	b, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=")
	return b
}
