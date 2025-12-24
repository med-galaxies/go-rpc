package main

import (
	"log"
	"net"
	"net/http"
	"time"
	"sync"
	"gorpc/server"
	"gorpc/xclient"
	"context"
	"os"
	"syscall"
	"os/signal"
)

type Foo int

type Args struct{ Num1, Num2 int }

func (f Foo) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

func (f Foo) Sleep(args Args, reply *int) error {
	time.Sleep(time.Second * time.Duration(args.Num1))
	*reply = args.Num1 + args.Num2
	return nil
}

func startServer(addr chan string) {
	s := server.NewServer()
	var foo Foo
	if err := s.Register(&foo); err != nil {
		log.Fatal("register error:", err)
	}
	// pick a free port
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("network error:", err)
	}
	log.Println("start rpc server on", l.Addr())

	hl, _ := net.Listen("tcp", ":0")
	log.Println("start http server on", hl.Addr())
	// s.HandleHTTP()
	// 创建独立的HTTP服务器
    mux := http.NewServeMux()
    mux.Handle(server.DefaultRPCPath, s)
	mux.Handle(server.DefaultDebugPath, server.DebugHTTP{s})
	addr <- l.Addr().String()
	// server.DefaultServer.Accept(l)
	// s.Accept
	
	go s.Accept(l)
	go http.Serve(hl, mux)
}

func foo(xc *xclient.XClient, ctx context.Context, typ, serviceMethod string, args *Args) {
	var reply int
	var err error
	switch typ {
	case "call":
		err = xc.Call(ctx, serviceMethod, args, &reply)
	case "broadcast":
		err = xc.Broadcast(ctx, serviceMethod, args, &reply)
	}
	if err != nil {
		log.Printf("%s %s error: %v", typ, serviceMethod, err)
	} else {
		log.Printf("%s %s success: %d + %d = %d", typ, serviceMethod, args.Num1, args.Num2, reply)
	}
}

func call(addr1, addr2 string) {
	d := xclient.NewMultiServerDiscovery([]string{"tcp@" + addr1, "tcp@" + addr2}, []int{1, 2}, 100, 50)
	xc := xclient.NewXClient(d, xclient.RandomSelect, nil)
	defer func() { _ = xc.Close() }()
	// send request & receive response
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			foo(xc, context.Background(), "call", "Foo.Sum", &Args{Num1: i, Num2: i * i})
		}(i)
	}
	wg.Wait()
}

func broadcast(addr1, addr2 string) {
	d := xclient.NewMultiServerDiscovery([]string{"tcp@" + addr1, "tcp@" + addr2}, []int{1, 2}, 100, 50)
	xc := xclient.NewXClient(d, xclient.WeightRoundRobinSelect, nil)
	defer func() { _ = xc.Close() }()
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			foo(xc, context.Background(), "broadcast", "Foo.Sum", &Args{Num1: i, Num2: i * i})
			// expect 2 - 5 timeout
			ctx, _ := context.WithTimeout(context.Background(), time.Second*2)
			foo(xc, ctx, "broadcast", "Foo.Sleep", &Args{Num1: i, Num2: i * i})
		}(i)
	}
	wg.Wait()
}

func main() {
	log.SetFlags(0)
	// addr := make(chan string)
	// go startServer(addr)

	// // in fact, following code is like a simple geerpc client
	// client, _ := client.DialHTTP("tcp", <-addr)
	// // defer func() { _ = client.Close() }()

	// time.Sleep(time.Second)

	// // _ = json.NewEncoder(conn).Encode(server.TestOption)

	// var wg sync.WaitGroup
	// for i := 0; i < 5; i++ {
	// 	wg.Add(1)
	// 	go func(i int) {
	// 		defer wg.Done()
	// 		args := &Args{Num1: i, Num2: i * i}
	// 		ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
	// 		var reply int
	// 		if err := client.Call(ctx, "Foo.Sum", args, &reply); err != nil {
	// 			log.Fatal("call testing error: ", err)
	// 		}
	// 		log.Printf("%d + %d = %d", args.Num1, args.Num2, reply)
	// 	}(i)
	// }
	// wg.Wait()
	ch1 := make(chan string)
	ch2 := make(chan string)
	// start two servers
	go startServer(ch1)
	go startServer(ch2)

	addr1 := <-ch1
	addr2 := <-ch2

	time.Sleep(time.Second)
	call(addr1, addr2)
	broadcast(addr1, addr2)

	quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    log.Println("Server shutting down...")
}