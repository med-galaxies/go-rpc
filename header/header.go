package rpcHeader

import (
	"io"
)

// Header represents the header structure for RPC communication.
type Header struct {
	ServiceMethod string // format: "Service.Method"
	Seq           uint64 // sequence number chosen by client
	Error         string // error message, empty if no error
}

type HeaderCodec interface {
	ReaderHeader(*Header) error
	ReaderBody(interface{}) error
	Writer(*Header) error
	io.Closer
}

type NewHeaderCodecFunc func(io.ReadWriteCloser) HeaderCodec

var NewHeaderCodecFuncMap map[string]NewHeaderCodecFunc

func init() {
	NewHeaderCodecFuncMap = make(map[string]NewHeaderCodecFunc)
	NewHeaderCodecFuncMap["gob"] = NewGobHeaderCodec
}
