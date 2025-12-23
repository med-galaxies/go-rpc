package xclient

import (
	"errors"
	"math"
	"math/rand"
	"sync"
	"time"
	"fmt"
	"crypto/md5"
	"sort"
	"encoding/binary"
	"strconv"
)

type SelectMode int

const (
	RandomSelect SelectMode = iota
	RoundRobinSelect
	WeightRoundRobinSelect
	ConsistentHashSelect
)

type Discovery interface {
	Refresh() error
	Update(servers []string) error
	Get(mode SelectMode) (string, error)
	GetAll() ([]string, error)
}

type MultiServerDiscovery struct {
	r       *rand.Rand
	mu      sync.RWMutex
	servers []string
	index   int
	currentWeights []int
	effectiveWeights []int
	maxWeight int
	consistentHash *ConsistentHash
}

type ConsistentHash struct {
	mu sync.RWMutex
	hashFunc func(data []byte) uint32
	replicas int
	keys []uint32
	hash2Server map[uint32]string
	servers []string
}

func NewConsistentHash(replicas int, servers []string) *ConsistentHash {
	if replicas <= 0 {
		replicas = 50
	}

	hash := ConsistentHash{
		replicas: replicas,
		hash2Server: make(map[uint32]string),
		servers: make([]string, len(servers)),
		hashFunc: defaultHash,
	}
	copy(hash.servers, servers)
	return &hash
}

func defaultHash(data []byte) uint32 {
	hash := md5.Sum(data)
	return binary.BigEndian.Uint32(hash[:4])
}

func (c *ConsistentHash) AddServer(server string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	isExist := false
	for _, v := range c.servers {
		if v == server {
			isExist = true
			break
		}
	}
	if isExist {
		return errors.New("rpc discovery: server already exist")
	}
	for i := 0; i < c.replicas; i++ {
		virtualKey := fmt.Sprintf("%s-%d", server, i)
		hash := c.hashFunc([]byte(virtualKey))
		c.keys = append(c.keys, hash)
		c.hash2Server[hash] = server
	}
	
	sort.Slice(c.keys, func(i, j int) bool {
		return c.keys[i] < c.keys[j]
	})
	c.servers = append(c.servers, server)
	return nil
}

func (c *ConsistentHash) RemoveServer(server string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	isExist := false
	for _, v := range c.servers {
		if v == server {
			isExist = true
			break
		}
	}
	if !isExist {
		return errors.New("rpc discovery: server not exist")
	}
	
	for i := 0; i < c.replicas; i++ {
		virtualKey := fmt.Sprintf("%s-%d", server, i)
		hash := c.hashFunc([]byte(virtualKey))
		delete(c.hash2Server, hash)
	}

	c.keys = c.keys[:0]
	for hash := range c.hash2Server {
		c.keys = append(c.keys, hash)
	}
	sort.Slice(c.keys, func(i, j int) bool {
		return c.keys[i] < c.keys[j]
	})
	c.servers = c.servers[:len(c.servers)-1]
	return nil
}

func (c *ConsistentHash) Get(key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.keys) == 0 {
		return "", errors.New("no servers available")
	}

	hash := c.hashFunc([]byte(key))
	idx := sort.Search(len(c.keys), func(i int) bool {
		return c.keys[i] >= hash
	})

	if idx == len(c.keys) {
		idx = 0
	}
	return c.hash2Server[c.keys[idx]], nil
}

func NewMultiServerDiscovery(servers []string, weights []int, maxWeight int, replicas int) *MultiServerDiscovery {
	d := &MultiServerDiscovery{
		servers: servers,
		r:       rand.New(rand.NewSource(time.Now().UnixNano())),
		effectiveWeights: make([]int, len(servers)),
		currentWeights: make([]int, len(servers)),
		maxWeight: maxWeight,
		consistentHash: NewConsistentHash(replicas, servers),
	}
	d.index = d.r.Intn(math.MaxInt32 - 1)
	copy(d.effectiveWeights, weights)
	
	return d
}

func (d *MultiServerDiscovery) Next() (string, error) {
	totalWeight := 0
	var selected string
	var selectedIdx int

	for idx, value := range d.servers {
		totalWeight += d.effectiveWeights[idx]
		d.currentWeights[idx] += d.effectiveWeights[idx]

		if selected == "" || d.currentWeights[idx] > d.currentWeights[selectedIdx] {
			selected = value
			selectedIdx = idx
		}
	}
	if selected == "" {
		return "", errors.New("no available servers")
	} 
	d.currentWeights[selectedIdx] -= totalWeight
	return selected, nil

}

func (d *MultiServerDiscovery) UpdateWeight(w int, name string) error {
	if w < 0 {
		return errors.New("rpc discovery: invalid index update")
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	for idx, value := range d.servers {
		if value == name {
			d.effectiveWeights[idx] = w
			d.currentWeights[idx] = 0
			return nil
		}
	}
	
	return fmt.Errorf("rpc discovery: server %s not exist", name)
}

func (d *MultiServerDiscovery) MarkSuccess(name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for idx, value := range d.servers {
		if value == name {
			d.currentWeights[idx] = min(d.maxWeight, d.currentWeights[idx] + 1)
			return nil
		}
	}
	return fmt.Errorf("rpc discovery: server %s not exist", name)
}

func (d *MultiServerDiscovery) MarkFailure(name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for idx, value := range d.servers {
		if value == name {
			d.currentWeights[idx] = max(0, d.currentWeights[idx] - 1)
			return nil
		}
	}
	return fmt.Errorf("rpc discovery: server %s not exist", name)
}

func (d *MultiServerDiscovery) Refresh() error {
	return nil
}

func (d *MultiServerDiscovery) Update(servers []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.servers = servers
	return nil
}

func (d *MultiServerDiscovery) Get(mode SelectMode) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := len(d.servers)
	if n == 0 {
		return "", errors.New("rpc discovery: no available servers")
	}
	switch mode {
	case RandomSelect:
		return d.servers[d.r.Intn(n)], nil
	case RoundRobinSelect:
		s := d.servers[d.index]
		d.index = (d.index + 1) % n
		return s, nil
	case WeightRoundRobinSelect:
		return d.Next()
	case ConsistentHashSelect:
		return d.consistentHash.Get(strconv.Itoa(d.r.Int()))
	default:
		return "", errors.New("rpc discovery: not supported select mode")
	}
}

func (d *MultiServerDiscovery) GetAll() ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	servers := make([]string, len(d.servers))
	copy(servers, d.servers)
	return servers, nil
}