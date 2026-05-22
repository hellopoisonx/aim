package framevue

import (
	"github.com/hellopoisonx/aim/app/frontend/wsclient"
)

// FrameEnvelope represents the frame envelope sent to the frontend.
type FrameEnvelope struct {
	Frame   *FrameInfo       `json:"frame,omitempty"`
	Payload map[string]any    `json:"payload,omitempty"`
}

// FrameInfo represents frame information.
type FrameInfo struct {
	Type int32  `json:"type,omitempty"`
	Seq  int64  `json:"seq,omitempty"`
}

// FromFrame converts a FramePayload to a FrameEnvelope for the frontend.
func FromFrame(fp *wsclient.FramePayload) *FrameEnvelope {
	if fp == nil {
		return nil
	}

	envelope := &FrameEnvelope{
		Frame: &FrameInfo{
			Type: int32(fp.Frame.Type),
			Seq:  fp.Frame.Seq,
		},
	}

	if fp.Payload != nil {
		envelope.Payload = fp.Payload.(map[string]any)
	}

	return envelope
}
