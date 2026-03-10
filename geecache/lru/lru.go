package lru

import "container/list"

type Cache struct {
	maxBytes  int64                         // 最大内存限制
	nbytes    int64                         // 当前已使用内存
	ll        *list.List                    // 用于维护访问顺序的双向链表
	cache     map[string]*list.Element      // 字典 用于O1查找
	onEvicted func(key string, value Value) // 记录被淘汰时的回调函数
}

type entry struct {
	key   string
	value Value
}

type Value interface {
	Len() int
}

// 新建缓存
func New(maxBytes int64, onEvicted func(string, Value)) *Cache {
	return &Cache{
		maxBytes:  maxBytes,
		ll:        list.New(),
		cache:     make(map[string]*list.Element),
		onEvicted: onEvicted,
	}
}

// 查找功能
func (c *Cache) Get(key string) (value Value, ok bool) {
	if ele, ok := c.cache[key]; ok {
		c.ll.MoveToFront(ele)
		kv := ele.Value.(*entry)
		return kv.value, true
	}
	return
}

// 删除
// 缓存淘汰 移除最近最少访问的节点（队首）
func (c *Cache) RemoveOldest() {
	ele := c.ll.Back()
	if ele != nil {
		c.ll.Remove(ele)
		kv := ele.Value.(*entry)
		delete(c.cache, kv.key)
		c.nbytes -= int64(len(kv.key)) + int64(kv.value.Len())
		if c.onEvicted != nil {
			c.onEvicted(kv.key, kv.value)
		}
	}
}

// 新增
// 如果键值存在则移至队头 更新数值
// 不存在则新增添加 最后检查是否超过最大值 若超过则移除
func (c *Cache) Add(key string, value Value) {
	if ele, ok := c.cache[key]; ok {
		c.ll.MoveToFront(ele)
		kv := ele.Value.(*entry)
		c.nbytes += int64(value.Len()) - int64(kv.value.Len())
		kv.value = value
	} else {
		ele := c.ll.PushFront(&entry{key, value})
		c.cache[key] = ele
		c.nbytes += int64(len(key)) + int64(value.Len())
	}
	for c.maxBytes != 0 && c.maxBytes < c.nbytes {
		c.RemoveOldest()
	}
}

// 测试使用 用于测试获取了多少条数据
func (c *Cache) Len() int {
	return c.ll.Len()
}
