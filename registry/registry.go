package registry

import (
	"time"
	"sync"
	"sort"
	"net/http"
	"log"
	"strings"
)

type GoRegistry struct {
	timeout time.Duration
	mu sync.Mutex
	servers map[string]*ServerItem
}

type ServerItem struct {
	Addr string
	start time.Time
}

const (
	defaultPath = "/_gorpc_/registry"
	defaultTimeout = time.Minute * 5
)

func NewGoRegistry(timeout time.Duration) *GoRegistry {
	return &GoRegistry{
		servers: make(map[string]*ServerItem),
		timeout: timeout,
	}
}

var DefaultGoRegistry = NewGoRegistry(defaultTimeout)

func (r *GoRegistry) putServer(addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.servers[addr]
	if ok {
		s.start = time.Now()
	} else {
		r.servers[addr] = &ServerItem{
			Addr: addr,
			start: time.Now(),
		}
	}
}

func (r *GoRegistry) aliveServers() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var alive []string
	for addr, s := range r.servers {
		if r.timeout == 0 || s.start.Add(r.timeout).After(time.Now()) {
			alive = append(alive, addr)
		} else {
			delete(r.servers, addr)
		}
	}
	sort.Strings(alive)
	return alive
}

func (r *GoRegistry) ServeHTTP(w http.ResponseWriter, req *http.Request){
	switch req.Method {
	case "GET":
		w.Header().Set("X-GoRPC-Servers", strings.Join(r.aliveServers(), ","))
	case "POST":
		addr := req.Header.Get("X-GoRPC-Server")
		if addr == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		r.putServer(addr)
		log.Println("put server:", addr)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (r *GoRegistry) HandleHTTP(registryPath string) {
	http.Handle(registryPath, r)
	log.Println("rpc registry path:", registryPath)
}

func HandleHTTP() {
	DefaultGoRegistry.HandleHTTP(defaultPath)
}

func HeartBeat(registry, addr string, duration time.Duration) {
	if duration == 0 {
		duration = defaultTimeout - time.Duration(1)*time.Minute
	}
	var err error
	err = sendHeartbeat(registry, addr)
	go func(){
		ticker := time.NewTicker(duration)
		for err == nil {
			<- ticker.C
			err = sendHeartbeat(registry, addr)
		}
	}()
}

func sendHeartbeat(registry, addr string) error {
	log.Println(addr, "send heart beat to registry", registry)
	httpClient := &http.Client{}
	req, _ := http.NewRequest("POST", registry, nil)
	req.Header.Set("X-GoRPC-Server", addr)
	if _, err := httpClient.Do(req); err != nil {
		log.Println("rpc server: heart beat err:", err)
		return err
	}
	return nil
}