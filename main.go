package main

import (
	"fmt"
	"log"
	"net"
	"time"
	"sync"
	"gorpc/server"
	"gorpc/client"
)

func startServer(addr chan string) {
	// pick a free port
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("network error:", err)
	}
	log.Println("start rpc server on", l.Addr())
	addr <- l.Addr().String()
	server.DefaultServer.Accept(l)
}

func main() {
	log.SetFlags(0)
	addr := make(chan string)
	go startServer(addr)

	// in fact, following code is like a simple geerpc client
	client, _ := client.Dial("tcp", <-addr)
	defer func() { _ = client.Close() }()

	time.Sleep(time.Second)
	// send options
	// _ = json.NewEncoder(conn).Encode(server.TestOption)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := fmt.Sprintf("gorpc req %d", i)
			var reply string
			if err := client.Call("testing", args, &reply); err != nil {
				log.Fatal("call testing error: ", err)
			}
			log.Println("Reply: ", reply)
		}(i)
	}
	wg.Wait()
}