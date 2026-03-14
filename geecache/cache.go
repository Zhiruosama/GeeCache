package geecache

import (
	"geecache/geecache/lru"
	"sync"
)

const defaultShardCount = 16

type cacheShard struct {
	mu       sync.Mutex
	lru      *lru.Cache
	maxBytes int64
}

type cache struct {
	initOnce   sync.Once
	shards     []cacheShard
	cacheBytes int64
}

func (c *cache) initShards() {
	shardCount := defaultShardCount
	if c.cacheBytes > 0 && c.cacheBytes < int64(shardCount) {
		shardCount = int(c.cacheBytes)
		if shardCount < 1 {
			shardCount = 1
		}
	}

	c.shards = make([]cacheShard, shardCount)
	if c.cacheBytes == 0 {
		return
	}

	perShard := c.cacheBytes / int64(shardCount)
	remainder := c.cacheBytes % int64(shardCount)
	for idx := range c.shards {
		c.shards[idx].maxBytes = perShard
		if int64(idx) < remainder {
			c.shards[idx].maxBytes++
		}
	}
}

func (c *cache) shardFor(key string) *cacheShard {
	c.initOnce.Do(c.initShards)
	idx := int(fnv32a(key) % uint32(len(c.shards)))
	return &c.shards[idx]
}

func fnv32a(key string) uint32 {
	var hash uint32 = 2166136261
	for idx := 0; idx < len(key); idx++ {
		hash ^= uint32(key[idx])
		hash *= 16777619
	}
	return hash
}

func (c *cache) add(key string, value ByteView) {
	shard := c.shardFor(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	if shard.lru == nil {
		shard.lru = lru.New(shard.maxBytes, nil)
	}
	shard.lru.Add(key, value)
}

func (c *cache) get(key string) (value ByteView, ok bool) {
	shard := c.shardFor(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	if shard.lru == nil {
		return
	}

	if v, ok := shard.lru.Get(key); ok {
		return v.(ByteView), ok
	}

	return
}
