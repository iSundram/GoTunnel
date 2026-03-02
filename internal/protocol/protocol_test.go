package protocol

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func TestEncodeDecodeFrame_RoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		frameType byte
		payload   []byte
	}{
		{"register", FrameRegister, []byte(`{"type":"register"}`)},
		{"heartbeat", FrameHeartbeat, []byte(`{"type":"heartbeat"}`)},
		{"open_stream", FrameOpenStream, []byte(`{"stream_id":"s1"}`)},
		{"stream_data", FrameStreamData, []byte("raw binary data")},
		{"close_stream", FrameCloseStream, []byte(`{"stream_id":"s1"}`)},
		{"control_response", FrameControlResponse, []byte(`{"code":1000}`)},
		{"empty_payload", FrameRegister, []byte{}},
		{"nil_payload", FrameHeartbeat, nil},
		{"large_payload", FrameStreamData, bytes.Repeat([]byte("x"), 1<<16)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := EncodeFrame(tt.frameType, tt.payload)

			// Verify header
			if frame[0] != tt.frameType {
				t.Fatalf("frame type: got %d, want %d", frame[0], tt.frameType)
			}
			length := binary.BigEndian.Uint32(frame[1:HeaderSize])
			if int(length) != len(tt.payload) {
				t.Fatalf("length field: got %d, want %d", length, len(tt.payload))
			}

			// Round-trip through DecodeFrame
			ft, payload, err := DecodeFrame(bytes.NewReader(frame))
			if err != nil {
				t.Fatalf("DecodeFrame error: %v", err)
			}
			if ft != tt.frameType {
				t.Fatalf("decoded type: got %d, want %d", ft, tt.frameType)
			}
			if !bytes.Equal(payload, tt.payload) {
				t.Fatalf("decoded payload mismatch")
			}
		})
	}
}

func TestDecodeFrame_TruncatedHeader(t *testing.T) {
	_, _, err := DecodeFrame(bytes.NewReader([]byte{0x01, 0x00}))
	if err == nil {
		t.Fatal("expected error for truncated header")
	}
}

func TestDecodeFrame_TruncatedPayload(t *testing.T) {
	header := make([]byte, HeaderSize)
	header[0] = FrameStreamData
	binary.BigEndian.PutUint32(header[1:], 100)
	_, _, err := DecodeFrame(bytes.NewReader(header))
	if err == nil {
		t.Fatal("expected error for truncated payload")
	}
}

func TestDecodeFrame_EmptyReader(t *testing.T) {
	_, _, err := DecodeFrame(bytes.NewReader(nil))
	if err == nil {
		t.Fatal("expected error for empty reader")
	}
}

func TestEncodeDecodeControlMessage_RoundTrip(t *testing.T) {
	original := Register{
		Type:     "register",
		Token:    "tok123",
		ClientID: "c1",
		Tunnels: []TunnelConfig{
			{Name: "web", Protocol: "http", Port: 8080, Subdomain: "myapp"},
		},
		Metadata: ClientMetadata{OS: "linux", Version: "1.0.0"},
	}

	frame, err := EncodeControlMessage(FrameRegister, original)
	if err != nil {
		t.Fatalf("EncodeControlMessage error: %v", err)
	}

	ft, payload, err := DecodeFrame(bytes.NewReader(frame))
	if err != nil {
		t.Fatalf("DecodeFrame error: %v", err)
	}
	if ft != FrameRegister {
		t.Fatalf("frame type: got %d, want %d", ft, FrameRegister)
	}

	var decoded Register
	if err := DecodeControlMessage(payload, &decoded); err != nil {
		t.Fatalf("DecodeControlMessage error: %v", err)
	}
	if decoded.Token != original.Token || decoded.ClientID != original.ClientID {
		t.Fatalf("decoded register mismatch")
	}
	if len(decoded.Tunnels) != 1 || decoded.Tunnels[0].Subdomain != "myapp" {
		t.Fatalf("decoded tunnels mismatch")
	}
}

func TestEncodeDecodeControlMessage_ControlResponse(t *testing.T) {
	original := ControlResponse{Code: CodeOK, Message: "success"}

	frame, err := EncodeControlMessage(FrameControlResponse, original)
	if err != nil {
		t.Fatalf("EncodeControlMessage error: %v", err)
	}

	ft, payload, err := DecodeFrame(bytes.NewReader(frame))
	if err != nil {
		t.Fatalf("DecodeFrame error: %v", err)
	}
	if ft != FrameControlResponse {
		t.Fatalf("frame type mismatch")
	}

	var decoded ControlResponse
	if err := DecodeControlMessage(payload, &decoded); err != nil {
		t.Fatalf("DecodeControlMessage error: %v", err)
	}
	if decoded.Code != CodeOK || decoded.Message != "success" {
		t.Fatalf("decoded control response mismatch")
	}
}

func TestDecodeControlMessage_InvalidJSON(t *testing.T) {
	var v Register
	if err := DecodeControlMessage([]byte("not json"), &v); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestEncodeControlMessage_UnmarshalableValue(t *testing.T) {
	// channels cannot be marshaled
	_, err := EncodeControlMessage(FrameRegister, make(chan int))
	if err == nil {
		t.Fatal("expected error for unmarshalable value")
	}
}

func TestMultipleFrames(t *testing.T) {
	var buf bytes.Buffer
	payloads := [][]byte{[]byte("first"), []byte("second"), []byte("third")}
	types := []byte{FrameRegister, FrameHeartbeat, FrameStreamData}

	for i, p := range payloads {
		buf.Write(EncodeFrame(types[i], p))
	}

	reader := bytes.NewReader(buf.Bytes())
	for i, want := range payloads {
		ft, got, err := DecodeFrame(reader)
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		if ft != types[i] {
			t.Fatalf("frame %d type: got %d, want %d", i, ft, types[i])
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("frame %d payload mismatch", i)
		}
	}
}

func TestFrameTypeConstants(t *testing.T) {
	seen := make(map[byte]string)
	types := map[byte]string{
		FrameRegister:        "Register",
		FrameHeartbeat:       "Heartbeat",
		FrameOpenStream:      "OpenStream",
		FrameStreamData:      "StreamData",
		FrameCloseStream:     "CloseStream",
		FrameControlResponse: "ControlResponse",
	}
	for v, name := range types {
		if prev, ok := seen[v]; ok {
			t.Fatalf("duplicate frame type value %d: %s and %s", v, prev, name)
		}
		seen[v] = name
	}
}

func TestErrorCodeConstants(t *testing.T) {
	codes := []uint32{CodeOK, CodeUnauthorized, CodeInvalidRequest, CodeTunnelNotFound, CodeStreamLimit, CodeInternalError}
	seen := make(map[uint32]bool)
	for _, c := range codes {
		if seen[c] {
			t.Fatalf("duplicate error code: %d", c)
		}
		seen[c] = true
	}
}

func TestDecodeFrame_ErrorWrapping(t *testing.T) {
	_, _, err := DecodeFrame(strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "reading frame header") {
		t.Fatalf("unexpected error message: %v", err)
	}
}
