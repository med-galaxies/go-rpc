package gorpc

import (
	"encoding/json"
	rpcHeader "gorpc/header"
	"io"
	"log"
	"net"
)

type Server struct{}

func NewServer() *Server {
	return &Server{}
}

type Option struct {
	MagicNumber     int
	HeaderCodecType string
}

const MagicNumber = 0x3bef5c

var DefaultOption = &Option{
	MagicNumber:     MagicNumber,
	HeaderCodecType: rpcHeader.HeaderCodecTypeGob,
}

func (s *Server) Accept(lis net.Listener) error {
	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Printf("rpc server: accept error: %v", err)
			return err
		}
		go s.ServeConn(conn)
	}
}

func (s *Server) ServeConn(conn io.ReadWriteCloser) {
	defer func() {
		_ = conn.Close()
	}()
	var opt Option
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Printf("rpc server: options error: %v", err)
		return
	}
	if opt.MagicNumber != MagicNumber {
		log.Printf("rpc server: invalid magic number %x", opt.MagicNumber)
		return
	}
	if opt.HeaderCodecType != rpcHeader.HeaderCodecTypeGob {
		log.Printf("rpc server: invalid header codec type %s", opt.HeaderCodecType)
		return
	}
	headerCodec := rpcHeader.NewHeaderCodecFuncMap[opt.HeaderCodecType](conn)
	//s.ServerConn(headerCodec);
	// TODO: Handle requests using headerCodec here.
	// For now, just log that the headerCodec was created.
	log.Printf("rpc server: headerCodec created: %T", headerCodec)

}
