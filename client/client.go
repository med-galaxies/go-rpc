package client

import (
	"encoding/json"
	"gorpc/codec"
	"gorpc/server"
	"sync"
	"fmt"
	"net"
	"log"
	"errors"
	"time"
	"context"
	"bufio"
	"io"
	"net/http"
	"strings"
)

type Call struct {
	Seq uint64
	ServiceMethod string
	Args interface{}
	Reply interface{}
	Error error
	Done chan * Call
}

func (c *Call) done() {
	c.Done <- c
} 


type Client struct {
	cc codec.MsgCodec
	opt *server.Option
	sending sync.Mutex
	header codec.Header
	mu sync.Mutex
	seq uint64
	pending map[uint64]*Call
	closing bool
	shutdown bool
}

var ShutDownErr = errors.New("connection is closing")

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing {
		return ShutDownErr
	}
	c.closing = true
	return c.cc.Close()
}

func (c *Client) IsAvailable() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closing && !c.shutdown
}

func (c *Client) registerCall(call *Call) (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing || c.shutdown {
		return 0, ShutDownErr
	}
	call.Seq = c.seq
	c.pending[c.seq] = call
	c.seq ++
	return call.Seq, nil
}

func (c *Client) removeCall(seq uint64) *Call {
	c.mu.Lock()
	defer c.mu.Unlock()
	call := c.pending[seq]
	delete(c.pending, seq)
	return call
}

func (c *Client) terminateCall(err error) {
	c.sending.Lock()
	defer c.sending.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.shutdown = true
	for _, call := range c.pending {
		call.Error = err
		call.done()
	}
}

func (c *Client) receive() {
	var err error
	for err == nil {
		var h codec.Header
		if err = c.cc.ReadHeader(&h); err != nil {
			break
		}
		call := c.removeCall(h.Seq)
		switch {
		case call == nil:
			err = c.cc.ReadBody(nil)
		case h.Error != "":
			call.Error = fmt.Errorf(h.Error)
			err = c.cc.ReadBody(nil)
			call.done()
		default:
			err = c.cc.ReadBody(call.Reply)
			if err != nil {
				call.Error = errors.New("reading body " + err.Error())
			}
			call.done()
		}
	}
	c.terminateCall(err)
}

func NewClient(conn net.Conn, opt *server.Option) (*Client, error) {
	f := codec.NewMsgCodecFuncMap[opt.CodecType]
	if f == nil {
		err := fmt.Errorf("invalid codec type %s", opt.CodecType)
		log.Println("rpc client: codec error ", err)
		return nil, err
	}
	if err := json.NewEncoder(conn).Encode(opt); err != nil {
		log.Println("rpc client: options error ", err)
		_ = conn.Close()
		return nil, err
	}
	return newClientCodec(f(conn), opt), nil
}

func newClientCodec(cc codec.MsgCodec, opt *server.Option) *Client {
	client := &Client {
		seq: 1,
		cc: cc,
		opt: opt,
		pending: make(map[uint64]*Call),
	}
	go client.receive()
	return client
}

func parseOptions(opts ...*server.Option) (*server.Option, error) {
	if len(opts) == 0 || opts[0] == nil {
		return server.DefaultOption, nil
	}
	if len(opts) > 1 {
		return nil, errors.New("number of options is more than 1")
	}
	opt := opts[0]
	opt.MagicNumber = server.DefaultOption.MagicNumber
	if opt.CodecType == "" {
		opt.CodecType = server.DefaultOption.CodecType
	}
	return opt, nil
}

type clientResult struct {
	client *Client
	err error
}

type newClientFunc func(conn net.Conn, opt *server.Option) (*Client, error)

func dialTimeout(f newClientFunc, network, address string, opts ...*server.Option) (client *Client, err error) {
	opt, err := parseOptions(opts...)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialTimeout(network, address, opt.ConnectTimeout)
	if err != nil {
		return nil, err
	}
	defer func() {
		if client == nil {
			conn.Close()
		}
	}()
	ch := make(chan clientResult, 1)
	go func() {
		client, err := f(conn, opt)
		ch <- clientResult{client, err}
	}()
	if opt.ConnectTimeout == 0 {
		result := <-ch
		return result.client, result.err
	}
	select {
		case <-time.After(opt.ConnectTimeout):
			return nil, fmt.Errorf("rpc client: connect timeout: expect within %s", opt.ConnectTimeout)
		case result := <-ch:
			return result.client, result.err
	}
	return NewClient(conn, opt)
}

func Dial(network, address string, opts ...*server.Option) (*Client, error) {
	return dialTimeout(NewClient, network, address, opts...)
}

func (c *Client) send(call *Call) {
	c.sending.Lock()
	defer c.sending.Unlock()

	seq, err := c.registerCall(call)
	if err != nil {
		call.Error = err
		call.done()
		return
	}

	c.header.ServiceMethod = call.ServiceMethod
	c.header.Seq = call.Seq
	c.header.Error = ""

	if err := c.cc.Write(&c.header, call.Args); err != nil {
		call := c.removeCall(seq)
		if call != nil {
			call.Error = err
			call.done()
		}
	}
}

func (c *Client) Go(serviceMethod string, args, reply interface{}, done chan *Call) *Call {
	if done == nil {
		done = make(chan *Call, 10)
	} else if cap(done) == 0{
		log.Panic("rpc client: done channel is unbuffered")
	}
	call := &Call {
		ServiceMethod: serviceMethod,
		Args: args,
		Reply: reply,
		Done: done,
	}
	c.send(call)
	return call
}

func (c *Client) Call(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	call := c.Go(serviceMethod, args, reply, make(chan *Call, 1))
	select {
	case <- ctx.Done():
		c.removeCall(call.Seq)
		return errors.New("rpc client: call failed: " + ctx.Err().Error())
	case call := <- call.Done:
		return call.Error
	}
}

func NewHTTPClient(conn net.Conn, opt *server.Option) (*Client, error) {
	io.WriteString(conn, fmt.Sprintf("CONNECT %s HTTP/1.0\n\n", server.DefaultRPCPath))

	// Require successful HTTP response
	// before switching to RPC protocol.
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err == nil && resp.Status == server.Connected {
		return NewClient(conn, opt)
	}
	if err == nil {
		err = errors.New("unexpected HTTP response: " + resp.Status)
	}
	return nil, err
}

func DialHTTP(network, address string, opts ...*server.Option) (*Client, error) {
	return dialTimeout(NewHTTPClient, network, address, opts...)
}

func XDial(rpcAddr string, opts ...*server.Option) (*Client, error) {
	parts := strings.Split(rpcAddr, "@")
	if len(parts) != 2 {
		return nil, fmt.Errorf("rpc client err: wrong format '%s', expect protocol@addr", rpcAddr)
	}
	protocol, addr := parts[0], parts[1]
	switch protocol {
	case "http":
		return DialHTTP("tcp", addr, opts...)
	default:
		// tcp
		return Dial(protocol, addr, opts...)
	}
}