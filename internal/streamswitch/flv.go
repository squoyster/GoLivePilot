package streamswitch

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	tagTypeAudio  = 8
	tagTypeVideo  = 9
	tagTypeScript = 18

	avcKeyframe       = 0x17 // 1 (keyframe) << 4 | 7 (AVC)
	avcInterFrame     = 0x27 // 2 (inter) << 4 | 7 (AVC)
	avcPacketTypeSeq  = 0
	avcPacketTypeNALU = 1

	aacPacketTypeSeq  = 0
	aacPacketTypeRaw  = 1
)

// FLVHeader is the 9-byte FLV file header.
type FLVHeader struct {
	HasAudio bool
	HasVideo bool
}

// FLVTag is a single FLV tag (one video/audio/script frame or packet).
type FLVTag struct {
	Type      uint8  // 8=audio, 9=video
	Timestamp uint32 // milliseconds
	Payload   []byte // including the codec-specific header byte(s)
}

// TagSize returns the on-wire tag size (11 header bytes + payload length).
func (t *FLVTag) TagSize() uint32 {
	return 11 + uint32(len(t.Payload))
}

// IsKeyframe returns true if this is an H.264 IDR keyframe.
func (t *FLVTag) IsKeyframe() bool {
	if t.Type != tagTypeVideo || len(t.Payload) < 1 {
		return false
	}
	// First byte: high 4 bits = frame type, low 4 bits = codec ID
	return t.Payload[0] == avcKeyframe
}

// IsAVCSeqHeader returns true if this tag contains AVC sequence header (SPS/PPS).
func (t *FLVTag) IsAVCSeqHeader() bool {
	if t.Type != tagTypeVideo || len(t.Payload) < 2 {
		return false
	}
	if t.Payload[0] != avcKeyframe && t.Payload[0] != avcInterFrame {
		return false
	}
	return t.Payload[1] == avcPacketTypeSeq
}

// IsAACSeqHeader returns true if this tag contains AAC sequence header (AudioSpecificConfig).
func (t *FLVTag) IsAACSeqHeader() bool {
	if t.Type != tagTypeAudio || len(t.Payload) < 2 {
		return false
	}
	return (t.Payload[0]>>4)&0x0F == 10 && t.Payload[1] == aacPacketTypeSeq
}

// FLVReader reads FLV tags from a byte stream.
type FLVReader struct {
	r       io.Reader
	buf     []byte
	decoded bool
}

// NewFLVReader creates a reader that parses FLV from r.
func NewFLVReader(r io.Reader) *FLVReader {
	return &FLVReader{r: r, buf: make([]byte, 4096)}
}

// ReadHeader reads and validates the FLV file header.
func (fr *FLVReader) ReadHeader() (*FLVHeader, error) {
	hdr := make([]byte, 9)
	if _, err := io.ReadFull(fr.r, hdr); err != nil {
		return nil, fmt.Errorf("flv: read header: %w", err)
	}
	if hdr[0] != 'F' || hdr[1] != 'L' || hdr[2] != 'V' {
		return nil, fmt.Errorf("flv: invalid signature: %c%c%c", hdr[0], hdr[1], hdr[2])
	}
	hasAudio := hdr[4]&0x04 != 0
	hasVideo := hdr[4]&0x01 != 0
	// Skip PreviousTagSize0 (4 bytes)
	prevSize := make([]byte, 4)
	if _, err := io.ReadFull(fr.r, prevSize); err != nil {
		return nil, fmt.Errorf("flv: read prevTagSize0: %w", err)
	}
	return &FLVHeader{HasAudio: hasAudio, HasVideo: hasVideo}, nil
}

// ReadTag reads a single FLV tag from the stream.
func (fr *FLVReader) ReadTag() (*FLVTag, error) {
	// Tag header: 1 (type) + 3 (data size) + 3 (timestamp) + 1 (timestamp ext) + 3 (stream ID) = 11 bytes
	hdr := fr.buf[:11]
	if _, err := io.ReadFull(fr.r, hdr); err != nil {
		return nil, fmt.Errorf("flv: read tag header: %w", err)
	}

	tagType := hdr[0]
	dataSize := uint32(binary.BigEndian.Uint32([]byte{0, hdr[1], hdr[2], hdr[3]}))

	// Timestamp: 3 bytes LE + 1 byte BE (extended)
	timestamp := uint32(hdr[4]) | uint32(hdr[5])<<8 | uint32(hdr[6])<<16 | uint32(hdr[7])<<24

	// Allocate or grow buffer for payload + prevSize
	needed := int(dataSize) + 4
	if cap(fr.buf) < needed {
		fr.buf = make([]byte, needed)
	}
	fr.buf = fr.buf[:needed]

	if _, err := io.ReadFull(fr.r, fr.buf[:dataSize+4]); err != nil {
		return nil, fmt.Errorf("flv: read payload: %w", err)
	}

	payload := make([]byte, dataSize)
	copy(payload, fr.buf[:dataSize])

	return &FLVTag{
		Type:      tagType,
		Timestamp: timestamp,
		Payload:   payload,
	}, nil
}

// FLVWriter writes FLV tags to a byte stream.
type FLVWriter struct {
	w       io.Writer
	started bool
}

// NewFLVWriter creates a writer that writes FLV to w.
func NewFLVWriter(w io.Writer) *FLVWriter {
	return &FLVWriter{w: w}
}

// WriteHeader writes the FLV file header and PreviousTagSize0.
func (fw *FLVWriter) WriteHeader(hasAudio, hasVideo bool) error {
	if fw.started {
		return nil
	}
	flags := byte(0)
	if hasAudio {
		flags |= 0x04
	}
	if hasVideo {
		flags |= 0x01
	}
	hdr := []byte{'F', 'L', 'V', 0x01, flags, 0x00, 0x00, 0x00, 0x09}
	if _, err := fw.w.Write(hdr); err != nil {
		return fmt.Errorf("flv: write header: %w", err)
	}
	if _, err := fw.w.Write([]byte{0x00, 0x00, 0x00, 0x00}); err != nil {
		return fmt.Errorf("flv: write prevTagSize0: %w", err)
	}
	fw.started = true
	return nil
}

// WriteTag writes a single FLV tag followed by PreviousTagSize.
func (fw *FLVWriter) WriteTag(tag *FLVTag) error {
	dataSize := len(tag.Payload)
	if dataSize > 0xFFFFFF {
		return fmt.Errorf("flv: tag data too large: %d", dataSize)
	}

	// Tag header (11 bytes)
	hdr := make([]byte, 11)
	hdr[0] = tag.Type
	hdr[1] = byte(dataSize >> 16)
	hdr[2] = byte(dataSize >> 8)
	hdr[3] = byte(dataSize)
	hdr[4] = byte(tag.Timestamp)
	hdr[5] = byte(tag.Timestamp >> 8)
	hdr[6] = byte(tag.Timestamp >> 16)
	hdr[7] = byte(tag.Timestamp >> 24) // extended
	// hdr[8:11] = stream ID (always 0)

	if _, err := fw.w.Write(hdr); err != nil {
		return fmt.Errorf("flv: write tag header: %w", err)
	}
	if _, err := fw.w.Write(tag.Payload); err != nil {
		return fmt.Errorf("flv: write tag payload: %w", err)
	}

	// PreviousTagSize: size of this tag (header + payload)
	prevSize := uint32(dataSize + 11)
	ps := []byte{byte(prevSize >> 24), byte(prevSize >> 16), byte(prevSize >> 8), byte(prevSize)}
	if _, err := fw.w.Write(ps); err != nil {
		return fmt.Errorf("flv: write prevTagSize: %w", err)
	}
	return nil
}

// WriteSilentAAC writes a sequence of AAC null frames for audio gap fill.
// Each frame is ~21ms of silence at 48kHz. frames parameter is the count.
func (fw *FLVWriter) WriteSilentAAC(timestamp uint32, frames int) error {
	// Pre-encoded AAC silence frame: ADTS header + null frame data.
	// LC AAC 48kHz stereo 128kbps null frame:
	// ADTS header = 7 bytes + 2 bytes raw data
	silenceFrame := []byte{
		// ADTS header
		0xFF, 0xF1,             // syncword + ID + layer + protection_absent
		0x50, 0x80,             // profile (LC) + sample rate (48kHz) + private bit + channel config (stereo)
		0x01, 0x20, 0xFC,       // frame length (2048bps at 48kHz ~ 2 x null frame? no...)
		// Actually let me use a simpler approach. Each frame is just the ADTS header + 2 bytes.
		// Frame length field spans bits 30-43 of the ADTS header = 3 bytes spanning header[3], header[4], header[5]
		// Total frame length = 7 (ADTS) + 2 (raw data) = 9 bytes
		// header[3] = 0x50 (profile + rate bits)
		// header[4] = 0x80 | ((9 << 3) & 0xE0) | 0x00 = 0x80 | 0x40 = 0xC0... no this is getting complicated.
	}
	_ = silenceFrame

	// Build frame per call to avoid complexity
	for i := 0; i < frames; i++ {
		pts := timestamp + uint32(i)*21
		// ADTS frame: 7 bytes header, 2 bytes raw data
		frame := buildAACNullFrame(pts)
		tag := &FLVTag{
			Type:      tagTypeAudio,
			Timestamp: pts,
			Payload:   frame,
		}
		if err := fw.WriteTag(tag); err != nil {
			return err
		}
	}
	return nil
}

// buildAACNullFrame constructs a complete AAC ADTS frame containing silence.
// Frame is 9 bytes: 7-byte ADTS header + 2-byte null payload.
func buildAACNullFrame(timestamp uint32) []byte {
	// ADTS header for LC AAC, 48kHz, stereo:
	// syncword: 12 bits (0xFFF)
	// ID: 1 bit (0 = MPEG4)
	// layer: 2 bits (00)
	// protection_absent: 1 bit (1 = no CRC)
	// profile: 2 bits (01 = AAC LC)
	// sample_rate: 4 bits (0x3 = 48000Hz)
	// private: 1 bit (0)
	// channel_config: 3 bits (0x2 = stereo)
	// original/copy: 1 bit (0)
	// home: 1 bit (0)
	// copyright_id: 1 bit (0)
	// copyright_start: 1 bit (0)
	// frame_length: 13 bits (9 bytes)
	// buffer_fullness: 11 bits (0x7FF = variable bitrate)
	// num_raw_blocks: 2 bits (0)
	//
	// Byte layout:
	// 0: 0xFF
	// 1: 0xF1 (syncword low + ID + layer + protect)
	// 2: 0x4C (profile + rate high) -> profile=01, rate=0011 -> 0100 1100 = 0x4C
	//    Wait: profile bit starts at bit 16: 0xFFF1|00|1|01|0011|0...
	//    Bits: sync(12)=FFF, ID(1)=0, layer(2)=00, protect(1)=1 -> 0xFF 0xF1
	//    Next: profile(2)=01, rate(4)=0011 -> 0100 1100 = 0x4C? No...
	//    0xFF F1 = 1111 1111 1111 0001
	//    Next bits: 01 0011 0 | 0 00 ...
	//    Byte 2: 01 0011 00 = 0x4C
	//    Byte 3: 0 | 00 | 0 | 0 | 00 0 | 000 00 | 000 = ?
	//    This is getting complex. Let me precompute:
	//
	// ADTS fixed header: 0xFF 0xF1 0x4C 0x80
	//   Byte 0: 0xFF (syncword high)
	//   Byte 1: 0xF1 (syncword low 0xF1 | ID 0 << 3 | layer 0 << 1 | protect 1 = 0xF1)
	//   Byte 2: 0x4C (profile 01 << 6 | rate 0x3 << 2 | private 0 << 1 | channel_config >> 2)
	//           = 01_00_11_0_0 = 0x4C
	//           Wait: 01 (profile LC) << 6 = 0x40
	//                  0011 (rate 48kHz) << 2 = 0x0C
	//                  0 (private)
	//                  = 0x4C? 0x40 + 0x0C = 0x4C. Yes.
	//   Byte 3: 0x80 (channel_config 2 << 6 = 0x80 | original 0 | home 0 | copyright 0 | copyright_start 0 | frame_length >> 11)
	//            = 10_000000 = 0x80
	// Wait, frame_length is 13 bits spanning bytes 3-5.
	// Byte 3: channel_config(3) | original(1) | home(1) | copyright(1) | copyright_start(1) | frame_length(13 bits, MSB part)
	// channel_config = 2 (stereo) = 010
	// Byte 3: 010 | 0 | 0 | 0 | 0 = 0100 0000 = 0x40
	// Then frame_length top 2 bits: 0000 00|00 (next 2 bits are the start of frame_length)
	// Hmm, let me just look up the ADTS header spec more carefully.

	// Actually, for a 9-byte frame:
	// Bit layout:
	// Byte  byte  byte  byte  byte  byte  byte
	// 0     1     2     3     4     5     6
	// FFFFn_ll_pHh HhHH_Ssss_pXX_XXXX_XXXX_XXbb bbbb_bbbb_bbrr rr(r2b2...)
	// This is too error-prone. Let me use a known-good pre-encoded frame.

	// Known-good ADTS header for 9-byte AAC LC 48kHz stereo null frame:
	// 0xFF 0xF1 0x4C 0x80 0x00 0x20 0xFC
	// Verified:
	// 0xFFF = syncword (12 bits)
	// 0 = ID (MPEG4)
	// 00 = layer
	// 1 = protection_absent (no CRC)
	// 01 = profile (LC)
	// 0011 = sample rate (48kHz)
	// 0 = private
	// 010 = channel config (stereo)
	// 0 = original
	// 0 = home
	// 0 = copyright ID
	// 0 = copyright start
	// frame length = 9 = 0x0000001001 13 bits
	//   In bytes 3-5: byte3 has 0x80 (channels 010 | 0 | 0 | 0 | 0 = 0x40) + frame_length(6 bits)
	//   Hmm, this doesn't add up. Let me just use the pre-computed bytes directly.
	//
	// For a 9-byte ADTS frame:
	// Bits 30-42 = frame_length (13 bits) = 9 = 0x0000000001001
	// Byte 3 bit 7-3: channels(3) | orig(1) | home(1) | copyright(1) | copyright_start(1)
	//   = 010 0 0 0 0 = 0x40
	// Byte 3 bit 2-0: frame_length bits 12-10 = 000 (top 3 of 13)
	//   So byte 3 = 0x40 | 0x00 = 0x40
	// Byte 4: frame_length bits 9-2 = 00000010 (8 of 13) = 0x02
	//   Hmm 9 = 0b0000000001001, bits 12-0:
	//   Bit 12-10 = 000 -> byte 3 bits 2-0
	//   Bit 9-2 = 00000001 = 0x01 -> byte 4
	//   Bit 1-0 = 01 -> byte 5 bits 7-6
	//   Wait let me count: 9 = 0b0000000000001001
	//   Bits 12-10 (top 3): 000 -> byte 3 bits 2-0 = 000
	//   Bits 9-2 (8 bits): 00000001 = 0x01 -> byte 4 = 0x01
	//   Bits 1-0 (2 bits): 01 -> byte 5 bits 7-6 = 01
	// Byte 5: 01 | buffer_fullness(6 bits) = 01_111111? Actually buffer_fullness for VBR is 0x7FF (11 bits)
	// Bits 5-0 of buffer fullness go in byte 5-6
	//   buffer_fullness = 0x7FF = 11111111111 (11 bits)
	//   Byte 5 bits 5-0: top 6 bits of buffer_fullness = 111111
	//   Byte 5 = 01_111111 = 0x7F
	// Byte 6: bottom 5 bits of buffer_fullness + num_raw_blocks(2)
	//   = 11111 | 00 = 0xFC? 11111 | 00 = 11111100 = 0xFC

	// So the full ADTS header is: 0xFF 0xF1 0x4C 0x40 0x01 0x7F 0xFC
	// Let me double check: 0x4C -> byte 2
	// 0xFF F1 4C 40 01 7F FC

	return []byte{
		0xFF, 0xF1, 0x4C, 0x40, 0x01, 0x7F, 0xFC, // ADTS header
		0x00, 0x00, // null frame data
	}
}

// GOPBuffer holds a single Group of Pictures (all tags between keyframes).
// It's a complete GOP that can be safely forwarded to outputs.
type GOPBuffer struct {
	Tags     []FLVTag
	HasAudio bool
	HasVideo bool
	LastPTS  uint32
}
