package streamswitch

import (
	"bytes"
	"testing"
)

func TestFLVHeaderWriteRead(t *testing.T) {
	var buf bytes.Buffer
	w := NewFLVWriter(&buf)

	if err := w.WriteHeader(true, true); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}

	r := NewFLVReader(&buf)
	hdr, err := r.ReadHeader()
	if err != nil {
		t.Fatalf("ReadHeader: %v", err)
	}
	if !hdr.HasAudio || !hdr.HasVideo {
		t.Errorf("expected audio+video header")
	}
}

func TestFLVTagRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	w := NewFLVWriter(&buf)
	_ = w.WriteHeader(true, true)

	tag := &FLVTag{
		Type:      tagTypeVideo,
		Timestamp: 42,
		Payload:   []byte{0x17, 0x01, 0x00, 0x00, 0x00, 0x05, 0x01, 0x02, 0x03, 0x04, 0x05},
	}
	if err := w.WriteTag(tag); err != nil {
		t.Fatalf("WriteTag: %v", err)
	}

	r := NewFLVReader(&buf)
	_, _ = r.ReadHeader()

	got, err := r.ReadTag()
	if err != nil {
		t.Fatalf("ReadTag: %v", err)
	}
	if got.Type != tagTypeVideo {
		t.Errorf("expected video tag, got %d", got.Type)
	}
	if got.Timestamp != 42 {
		t.Errorf("expected timestamp 42, got %d", got.Timestamp)
	}
	if !bytes.Equal(got.Payload, tag.Payload) {
		t.Errorf("payload mismatch")
	}
}

func TestKeyframeDetection(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		isKey   bool
	}{
		{
			name:    "AVC keyframe",
			payload: []byte{0x17, 0x01, 0x00, 0x00, 0x00},
			isKey:   true,
		},
		{
			name:    "AVC inter frame",
			payload: []byte{0x27, 0x01, 0x00, 0x00, 0x00},
			isKey:   false,
		},
		{
			name:    "audio tag",
			payload: []byte{0xAF, 0x01, 0x00},
			isKey:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tag := &FLVTag{Type: tagTypeVideo, Payload: tt.payload}
			if tt.name == "audio tag" {
				tag.Type = tagTypeAudio
			}
			if got := tag.IsKeyframe(); got != tt.isKey {
				t.Errorf("IsKeyframe = %v, want %v", got, tt.isKey)
			}
		})
	}
}

func TestGOPBuffer(t *testing.T) {
	gop := GOPBuffer{
		Tags: []FLVTag{
			{Type: tagTypeVideo, Timestamp: 0, Payload: []byte{0x17, 0x00, 0x01}},
			{Type: tagTypeVideo, Timestamp: 33, Payload: []byte{0x27, 0x00, 0x02}},
			{Type: tagTypeAudio, Timestamp: 43, Payload: []byte{0xAF, 0x01}},
		},
		HasVideo: true,
		HasAudio: true,
		LastPTS:  43,
	}

	if len(gop.Tags) != 3 {
		t.Errorf("expected 3 tags, got %d", len(gop.Tags))
	}
	if gop.LastPTS != 43 {
		t.Errorf("expected LastPTS 43, got %d", gop.LastPTS)
	}
}

func TestAACNullFrame(t *testing.T) {
	frame := buildAACNullFrame(0)
	if len(frame) != 9 {
		t.Errorf("expected 9-byte AAC frame, got %d", len(frame))
	}
	// ADTS syncword should be 0xFFF (12 bits)
	if frame[0] != 0xFF && frame[1]&0xF0 != 0xF0 {
		t.Errorf("expected ADTS syncword 0xFFF, got 0x%02X 0x%02X", frame[0], frame[1])
	}
}

func TestSilentAACWrite(t *testing.T) {
	var buf bytes.Buffer
	w := NewFLVWriter(&buf)
	_ = w.WriteHeader(true, true)

	if err := w.WriteSilentAAC(100, 2); err != nil {
		t.Fatalf("WriteSilentAAC: %v", err)
	}

	r := NewFLVReader(&buf)
	_, _ = r.ReadHeader()

	tag, err := r.ReadTag()
	if err != nil {
		t.Fatalf("ReadTag after silence: %v", err)
	}
	if tag.Type != tagTypeAudio {
		t.Errorf("expected audio tag, got %d", tag.Type)
	}
	if tag.Timestamp != 100 {
		t.Errorf("expected timestamp 100, got %d", tag.Timestamp)
	}
}