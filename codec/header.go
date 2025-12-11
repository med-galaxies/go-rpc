package codec

import (
	"io"
)

// Header represents the header structure for RPC communication.
type Header struct {
	ServiceMethod string // format: "Service.Method"
	Seq           uint64 // sequence number chosen by client
	Error         string // error message, empty if no error
}

type MsgCodec interface {
	ReadHeader(*Header) error
	ReadBody(interface{}) error
	Write(*Header, interface{}) error
	io.Closer
}

const (
	GobType string = "application/gob"
	JsonType string = "application/json"
)

type NewMsgCodecFunc func(io.ReadWriteCloser) MsgCodec

var NewMsgCodecFuncMap map[string]NewMsgCodecFunc

func init() {
	NewMsgCodecFuncMap = make(map[string]NewMsgCodecFunc)
	NewMsgCodecFuncMap[GobType] = NewGobCodec
	NewMsgCodecFuncMap[JsonType] = NewJsonCodec
}
