package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// Frame type constants.
const (
	FrameRegister        byte = 1
	FrameHeartbeat       byte = 2
	FrameOpenStream      byte = 3
	FrameStreamData      byte = 4
	FrameCloseStream     byte = 5
	FrameControlResponse byte = 6
)

// Error code constants.
const (
	CodeOK             uint32 = 1000
	CodeUnauthorized   uint32 = 2001
	CodeInvalidRequest uint32 = 2002
	CodeTunnelNotFound uint32 = 3001
	CodeStreamLimit    uint32 = 3002
	CodeInternalError  uint32 = 4001
)

// HeaderSize is the size of the frame header in bytes (1 type + 4 length).
const HeaderSize = 5

// TunnelConfig describes a single tunnel in a Register message.
type TunnelConfig struct {
	Name      string `json:"name"`
	Protocol  string `json:"protocol"`
	Port      int    `json:"port"`
	Subdomain string `json:"subdomain"`
}

// ClientMetadata carries OS and version info in a Register message.
type ClientMetadata struct {
	OS      string `json:"os"`
	Version string `json:"version"`
}

// Register is sent by the client to register tunnels.
type Register struct {
	Type     string         `json:"type"`
	Token    string         `json:"token"`
	Tunnels  []TunnelConfig `json:"tunnels"`
	ClientID string         `json:"client_id"`
	Metadata ClientMetadata `json:"metadata"`
}

// Heartbeat is a keep-alive message.
type Heartbeat struct {
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`
}

// StreamMetadata carries HTTP method and path for an opened stream.
type StreamMetadata struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

// OpenStream tells the client to open a new stream to a target tunnel.
type OpenStream struct {
	Type         string         `json:"type"`
	StreamID     string         `json:"stream_id"`
	TargetTunnel string         `json:"target_tunnel"`
	Metadata     StreamMetadata `json:"metadata"`
}

// StreamData carries raw bytes associated with a stream.
type StreamData struct {
	StreamID string `json:"stream_id"`
	Data     []byte `json:"data"`
}

// CloseStream signals that a stream should be closed.
type CloseStream struct {
	StreamID string `json:"stream_id"`
}

// ControlResponse is a generic server response.
type ControlResponse struct {
	Code    uint32      `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// EncodeFrame encodes a complete binary frame: [type(1)][length(4)][payload(N)].
func EncodeFrame(frameType byte, payload []byte) []byte {
	frame := make([]byte, HeaderSize+len(payload))
	frame[0] = frameType
	binary.BigEndian.PutUint32(frame[1:HeaderSize], uint32(len(payload)))
	copy(frame[HeaderSize:], payload)
	return frame
}

// DecodeFrame reads a single frame from reader and returns its type and payload.
func DecodeFrame(reader io.Reader) (frameType byte, payload []byte, err error) {
	header := make([]byte, HeaderSize)
	if _, err = io.ReadFull(reader, header); err != nil {
		return 0, nil, fmt.Errorf("reading frame header: %w", err)
	}
	frameType = header[0]
	length := binary.BigEndian.Uint32(header[1:HeaderSize])
	payload = make([]byte, length)
	if length > 0 {
		if _, err = io.ReadFull(reader, payload); err != nil {
			return 0, nil, fmt.Errorf("reading frame payload: %w", err)
		}
	}
	return frameType, payload, nil
}

// EncodeControlMessage JSON-marshals msg and wraps it in a binary frame.
func EncodeControlMessage(frameType byte, msg interface{}) ([]byte, error) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshaling control message: %w", err)
	}
	return EncodeFrame(frameType, payload), nil
}

// DecodeControlMessage JSON-unmarshals a frame payload into v.
func DecodeControlMessage(payload []byte, v interface{}) error {
	if err := json.Unmarshal(payload, v); err != nil {
		return fmt.Errorf("unmarshaling control message: %w", err)
	}
	return nil
}
