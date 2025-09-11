package rpcHeader

import (
	"bufio"
	"encoding/gob"
	"io"
)

type GobHeaderCodec struct {
	conn io.ReadWriteCloser
	dec  *gob.Decoder
	enc  *gob.Encoder
	buf  *bufio.Writer
}

func (c *GobHeaderCodec) ReaderHeader(h *Header) error {
	return c.dec.Decode(h)
}

func (c *GobHeaderCodec) ReaderBody(body interface{}) error {
	return c.dec.Decode(body)
}

func (c *GobHeaderCodec) Writer(h *Header) error {
	return c.enc.Encode(h)
}

func (c *GobHeaderCodec) Close() error {
	return c.conn.Close()
}

func NewGobHeaderCodec(conn io.ReadWriteCloser) HeaderCodec {
	buf := bufio.NewWriter(conn)
	return &GobHeaderCodec{
		conn: conn,
		dec:  gob.NewDecoder(conn),
		enc:  gob.NewEncoder(buf),
		buf:  buf,
	}
}
