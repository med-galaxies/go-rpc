package client

import (
    "testing"
    "runtime"
    "net"
    "os"
    "gorpc/server"
	"context"
    "time"
    "sync"
)

// 测试用的服务类型
type TestService int

func (t *TestService) Echo(args string, reply *string) error {
    *reply = "echo: " + args
    return nil
}

func _assert(t *testing.T, condition bool, msg string, v ...interface{}) {
    if !condition {
        t.Errorf("assertion failed: "+msg, v...)
    }
}

func TestXDial(t *testing.T) {
    if runtime.GOOS == "linux" {
        addr := "/tmp/geerpc.sock"
        
        // 清理可能存在的socket文件
        _ = os.Remove(addr)
        defer func() {
            _ = os.Remove(addr) // 测试后清理
        }()
        
        var wg sync.WaitGroup
        wg.Add(1)
        
        go func() {
            defer wg.Done()
            
            // 注册测试服务
            var testSvc TestService
            if err := server.Register(&testSvc); err != nil {
                t.Errorf("register error: %v", err)
                return
            }
            
            l, err := net.Listen("unix", addr)
            if err != nil {
                t.Errorf("failed to listen unix socket: %v", err)
                return
            }
            defer l.Close()
            
            // 等待一小段时间确保服务器启动
            time.Sleep(100 * time.Millisecond)
            
            // 使用Accept而不是Accept来避免阻塞
            server.DefaultServer.Accept(l)
        }()
        
        // 等待服务器启动
        time.Sleep(200 * time.Millisecond)
        
        // 测试连接
        client, err := XDial("unix@" + addr)
        _assert(t, err == nil, "failed to connect unix socket: %v", err)
        
        if client != nil {
            defer client.Close()
            
            // 测试RPC调用
            ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
            defer cancel()
            
            var reply string
            args := "hello"
            err = client.Call(ctx, "TestService.Echo", args, &reply)
            _assert(t, err == nil, "failed to call Echo: %v", err)
            _assert(t, reply == "echo: hello", "unexpected reply: %s", reply)
        }
        
        wg.Wait()
    }
}

// 添加更多测试用例
func TestXDialInvalidProtocol(t *testing.T) {
    _, err := XDial("invalid@localhost:8080")
    if err == nil {
        t.Error("expected error for invalid protocol")
    }
}

func TestXDialInvalidFormat(t *testing.T) {
    testCases := []string{
        "invalidformat",        // 没有@分隔符
        "too@many@parts",       // 多个@分隔符
        "@onlyprotocol",        // 只有协议
        "onlyaddr@",           // 只有地址
    }
    
    for _, addr := range testCases {
        _, err := XDial(addr)
        if err == nil {
            t.Errorf("expected error for invalid address format: %s", addr)
        }
    }
}
