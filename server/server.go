package server

import (
	"net"
	"io"
	"gorpc/codec"
	"reflect"
	"log"
	"sync"
	"encoding/json"
	"errors"
	"time"
	"fmt"
	"strings"
)

const MagicNumber = 0x3bef5c

type Option struct {
	MagicNumber     int
	CodecType       string
	ConnectTimeout time.Duration
	HandleTimeout time.Duration
}

type request struct {
	h *codec.Header
	argv, replyv reflect.Value
	mtype *methodType
	svc *Service
}

var DefaultOption = &Option{
	MagicNumber:     MagicNumber,
	CodecType:       codec.GobType,
	ConnectTimeout: 10 * time.Second,
}

var TestOption = &Option{
	MagicNumber: MagicNumber,
	CodecType: codec.JsonType,
	ConnectTimeout: 10 * time.Second,
}

type Server struct {
	serviceMap sync.Map
}

func (s *Server) Register(rcvr interface{}) error {
	ns := NewService(rcvr)
	if _, dup := s.serviceMap.LoadOrStore(ns.name, ns); dup {
		return errors.New("rpc: service already defined: " + ns.name)
	}
	return nil
}

func (s *Server) findService(serviceMethod string) (service *Service, mtype *methodType, err error) {
	dot := strings.LastIndex(serviceMethod, ".")
	if dot < 0 {
		err = errors.New("rpc: service/method request ill-formed: " + serviceMethod)
		return
	}
	serviceName, methodName := serviceMethod[:dot], serviceMethod[dot+1:]
	svc, ok := s.serviceMap.Load(serviceName)
	if !ok {
		err = errors.New("rpc: service " + serviceName + " not found")
		return
	}
	service = svc.(*Service)
	mtype = service.method[methodName]
	if mtype == nil {
		err = errors.New("rpc: method " + methodName + " not found")
	}
	return
}

func Register(rcvr interface{}) error {
	return DefaultServer.Register(rcvr)
}

func NewServer() *Server {
	return &Server{}
}

var DefaultServer = NewServer()

func (s *Server) Accept(lis net.Listener) {
	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Printf("rpc server: accept error: %v", err)
			return
		}
		go s.ServeConn(conn)
	}
}

func Accept(lis net.Listener) {
	DefaultServer.Accept(lis)
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
	codec := codec.NewMsgCodecFuncMap[opt.CodecType]
	if codec == nil {
		log.Printf("rpc server: invalid codec type %s", opt.CodecType)
		return
	}
	s.serveCodec(codec(conn), &opt)
}

var invalidRequest = struct{}{}

func (s *Server) serveCodec(cc codec.MsgCodec, opt *Option) {
	sending := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	for {
		req, err := s.readRequest(cc)
		if err != nil {
			if req == nil {
				break
			}
			req.h.Error = err.Error()
			s.sendResponse(cc, req.h, invalidRequest, sending)
			continue
		}
		wg.Add(1)
		go s.handleRequest(cc, req, sending, wg, opt.HandleTimeout)
	}
	wg.Wait()
	cc.Close()
}

func (s *Server) readRequestHeader(cc codec.MsgCodec) (*codec.Header, error) {
	var h codec.Header
	if err := cc.ReadHeader(&h); err != nil {
		if err != io.EOF && err != io.ErrUnexpectedEOF {
			log.Println("rpc server: read header error:", err)
		}
		return nil, err
	}
	return &h, nil
}

func (s *Server) readRequest(cc codec.MsgCodec) (*request, error) {
	h, err := s.readRequestHeader(cc)
	if err != nil {
		return nil, err
	}
	req := &request{h: h}
	req.svc, req.mtype, err = s.findService(h.ServiceMethod)
	if err != nil {
		return req, err
	}
	req.argv = req.mtype.newArgv()
	req.replyv = req.mtype.newReplyv()

	argvi := req.argv.Interface()
	if req.argv.Type().Kind() != reflect.Ptr {
		argvi = req.argv.Addr().Interface()
	}

	if err = cc.ReadBody(argvi); err != nil {
		log.Println("rpc server: read argv err:", err)
	}
	return req, nil
}

func (s *Server) sendResponse(cc codec.MsgCodec, h *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	if err := cc.Write(h, body); err != nil {
		log.Println("rpc server: write response error:", err)
	}
}

func (s *Server) handleRequest(cc codec.MsgCodec, req *request, sending *sync.Mutex, wg *sync.WaitGroup, timeout time.Duration) {
	defer wg.Done()
	called := make(chan struct{})
	sent := make(chan struct{})
	go func() {
		err := req.svc.call(req.mtype, req.argv, req.replyv)
		called <- struct{}{}
		if err != nil {
			req.h.Error = err.Error()
			s.sendResponse(cc, req.h, invalidRequest, sending)
			sent <- struct{}{}
			return
		}
		s.sendResponse(cc, req.h, req.replyv.Interface(), sending)
		sent <- struct{}{}
	}()
	
	if timeout == 0{
		<-called
		<-sent
		return
	} else {
		select {
		case <-time.After(timeout):
			req.h.Error = fmt.Sprintf("rpc server: request handle timeout: expect within %s", timeout)
			s.sendResponse(cc, req.h, invalidRequest, sending)
		case <-called:
			<-sent
		}
		
	}
}