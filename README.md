# GoRPC

一个用Go语言实现的轻量级RPC（远程过程调用）框架，支持多种传输协议、编码格式和服务发现机制。

## 特性

- 🚀 **高性能**：基于Go语言的并发特性，支持高并发RPC调用
- 🔌 **多协议支持**：支持TCP和HTTP传输协议
- 📦 **多编码格式**：支持Gob和JSON编码
- 🔍 **服务发现**：内置注册中心和服务发现机制
- ⚖️ **负载均衡**：支持随机、轮询、加权轮询、一致性哈希等多种负载均衡策略
- 🐛 **调试支持**：内置Web调试界面，实时查看服务状态和调用统计
- ❤️ **健康检查**：自动心跳检测和故障节点剔除

## 项目结构

```
gorpc/
├── main.go                 # 主程序入口，包含示例代码
├── server/                 # RPC服务端实现
│   ├── server.go          # 核心服务器逻辑
│   ├── service.go         # 服务注册和管理
│   └── debug.go           # Web调试界面
├── client/                 # RPC客户端实现
│   ├── client.go          # 核心客户端逻辑
│   └── client_test.go     # 客户端测试
├── xclient/                # 增强型客户端
│   ├── xclient.go         # 支持负载均衡的客户端
│   ├── discovery.go       # 服务发现接口
│   └── discovery_registry.go # 注册中心发现实现
├── registry/               # 服务注册中心
│   └── registry.go        # 注册中心实现
└── codec/                  # 编码解码器
    ├── header.go          # 消息头定义
    ├── codec_gob.go       # Gob编码实现
    └── codec_json.go      # JSON编码实现
```

## 快速开始

### 1. 基本RPC服务

```go
package main

import (
    "log"
    "gorpc/server"
)

// 定义服务
type Foo int

type Args struct{ Num1, Num2 int }

func (f Foo) Sum(args Args, reply *int) error {
    *reply = args.Num1 + args.Num2
    return nil
}

func main() {
    // 创建服务器
    s := server.NewServer()
    
    // 注册服务
    var foo Foo
    if err := s.Register(&foo); err != nil {
        log.Fatal("register error:", err)
    }
    
    // 启动服务
    l, err := net.Listen("tcp", ":8080")
    if err != nil {
        log.Fatal("network error:", err)
    }
    log.Println("start rpc server on", l.Addr())
    s.Accept(l)
}
```

### 2. 客户端调用

```go
package main

import (
    "log"
    "gorpc/client"
)

func main() {
    // 建立连接
    client, err := client.Dial("tcp", "localhost:8080")
    if err != nil {
        log.Fatal("dial error:", err)
    }
    defer client.Close()
    
    // 调用RPC方法
    var reply int
    args := &Args{Num1: 10, Num2: 20}
    err = client.Call(context.Background(), "Foo.Sum", args, &reply)
    if err != nil {
        log.Fatal("call error:", err)
    }
    log.Printf("10 + 20 = %d", reply)
}
```

### 3. 使用服务发现

```go
// 启动注册中心
go startRegistry()

// 服务器注册到注册中心
registry.HeartBeat("http://localhost:9999/_gorpc_/registry", "tcp@localhost:8080", 0)

// 客户端通过注册中心发现服务
d := xclient.NewGoRegistryDiscovery("http://localhost:9999/_gorpc_/registry", 0)
xc := xclient.NewXClient(d, xclient.RandomSelect, nil)

// 调用RPC方法
var reply int
err := xc.Call(context.Background(), "Foo.Sum", args, &reply)
```

## 高级功能

### 负载均衡策略

```go
// 随机选择
xc := xclient.NewXClient(d, xclient.RandomSelect, nil)

// 轮询选择
xc := xclient.NewXClient(d, xclient.RoundRobinSelect, nil)

// 加权轮询
xc := xclient.NewXClient(d, xclient.WeightRoundRobinSelect, nil)

// 一致性哈希
xc := xclient.NewXClient(d, xclient.ConsistentHashSelect, nil)
```

### 广播调用

```go
// 向所有服务器广播请求
err := xc.Broadcast(context.Background(), "Foo.Sum", args, &reply)
```

### Web调试界面

启动服务器后，可以通过浏览器访问调试页面：

```
http://localhost:port/debug/gorpc
```

调试页面显示：
- 已注册的服务列表
- 各服务的方法签名
- 方法调用次数统计
- 服务运行状态

## 编码格式

### Gob编码（默认）

```go
// 使用Gob编码
client, err := client.Dial("tcp", "localhost:8080", &server.Option{
    CodecType: codec.GobType,
})
```

### JSON编码

```go
// 使用JSON编码
client, err := client.Dial("tcp", "localhost:8080", &server.Option{
    CodecType: codec.JsonType,
})
```

## 传输协议

### TCP传输

```go
// 直接TCP连接
client, err := client.Dial("tcp", "localhost:8080")
```

### HTTP传输

```go
// 通过HTTP CONNECT方法
client, err := client.DialHTTP("tcp", "localhost:8080")
```

## 配置选项

```go
option := &server.Option{
    MagicNumber:     0x3bef5c,           // 魔数，用于协议验证
    CodecType:       codec.GobType,       // 编码类型
    ConnectTimeout:  10 * time.Second,    // 连接超时
    HandleTimeout:   0,                   // 处理超时，0表示无限制
}
```

## 运行示例

```bash
# 运行主程序（包含所有示例）
go run main.go

# 程序将：
# 1. 启动注册中心（端口9999）
# 2. 启动两个RPC服务器
# 3. 执行各种RPC调用示例
# 4. 启动Web调试界面
```

## 测试

```bash
# 运行测试
go test ./...

# 运行客户端测试
go test ./client
```

## 性能特点

- **并发安全**：所有组件都是并发安全的
- **内存效率**：使用对象池和缓冲区复用
- **网络优化**：支持连接池和长连接
- **自动重试**：支持自动重试和故障转移

## 注意事项

1. **服务发现**：使用注册中心时，确保注册中心先启动
2. **心跳机制**：服务会定期发送心跳，超时服务会被自动剔除
3. **端口选择**：使用`:0`可以自动选择可用端口
4. **调试模式**：生产环境建议关闭Web调试界面
