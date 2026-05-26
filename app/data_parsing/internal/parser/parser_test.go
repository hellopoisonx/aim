package parser

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestDefaultParserImageGeneratesThumbnail(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 1024, 512))
	for y := 0; y < src.Bounds().Dy(); y++ {
		for x := 0; x < src.Bounds().Dx(); x++ {
			src.SetRGBA(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatalf("encode source: %v", err)
	}

	res, err := DefaultParser{}.Parse(context.Background(), "image", "image/png", buf.Bytes())
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if res.Width != 1024 || res.Height != 512 {
		t.Fatalf("original dimensions = %dx%d, want 1024x512", res.Width, res.Height)
	}
	if res.ThumbWidth != 512 || res.ThumbHeight != 256 {
		t.Fatalf("thumbnail dimensions = %dx%d, want 512x256", res.ThumbWidth, res.ThumbHeight)
	}
	if res.ThumbMIME != thumbMIMEPNG {
		t.Fatalf("ThumbMIME = %q, want %q", res.ThumbMIME, thumbMIMEPNG)
	}
	thumb, err := png.Decode(bytes.NewReader(res.Thumbnail))
	if err != nil {
		t.Fatalf("decode thumbnail: %v", err)
	}
	if got := thumb.Bounds(); got.Dx() != 512 || got.Dy() != 256 {
		t.Fatalf("decoded thumbnail bounds = %dx%d, want 512x256", got.Dx(), got.Dy())
	}
	if res.Metadata["format"] != "png" {
		t.Fatalf("metadata format = %v, want png", res.Metadata["format"])
	}
}

func TestDefaultParserImageFallsBackWhenUnsupported(t *testing.T) {
	res, err := DefaultParser{}.Parse(context.Background(), "image", "image/webp", []byte("not a supported image"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if res.ThumbWidth != 1 || res.ThumbHeight != 1 {
		t.Fatalf("fallback thumbnail dimensions = %dx%d, want 1x1", res.ThumbWidth, res.ThumbHeight)
	}
	if res.Metadata["parser"] != metadataParserPlaceholder {
		t.Fatalf("parser metadata = %v, want %s", res.Metadata["parser"], metadataParserPlaceholder)
	}
	if _, err := png.Decode(bytes.NewReader(res.Thumbnail)); err != nil {
		t.Fatalf("decode fallback thumbnail: %v", err)
	}
}
