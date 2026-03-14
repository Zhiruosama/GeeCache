# GeeCache

分布式缓存 GeeCache 学习项目。

## 压测环境与口径

- 接口：`http://localhost:9999/api?key=<key>`
- 节点：`8001/8002/8003`（`8003` 同时开启 API）
- 压测参数：并发 `200`，总请求 `10000` 或 `20000`
- 机器：本机 Windows（AMD Ryzen 7 7735H）

## 第一轮（原始版本）

- 总请求：`10000`
- 总耗时：`0.362s`
- QPS：`27636.38`
- 延迟：`avg 7.16ms / p50 5.56ms / p95 11.16ms / p99 63.32ms / max 82.64ms`

## 第二轮（首批优化后）

### 优化点

- 去掉缓存命中高频日志（`geecache/geecache.go`）
- `PickPeer` 改读锁，减少锁竞争（`geecache/http.go`）
- 复用 `http.Client` + KeepAlive（`geecache/http.go`）
- 修复远程 URL 拼接与错误返回（`geecache/http.go`）
- 新增并行基准测试（`geecache/benchmark_test.go`）

### 结果（同口径）

- 总请求：`10000`
- 总耗时：`0.300s`
- QPS：`33283.64`
- 延迟：`avg 5.61ms / p50 3.78ms / p95 8.72ms / p99 60.39ms / max 101.42ms`

> 对比第一轮：QPS 约 `+20.4%`，avg/p95 延迟约 `-21%`。

## 第三轮（LRU 分片后）

### 优化点

- 本地缓存从单实例改为 `16` 分片（每片独立 `LRU + Mutex`）：`geecache/cache.go`
- 总容量按分片均分，保持总容量上限语义：`geecache/cache.go`
- `key -> shard` 使用无分配 FNV 哈希：`geecache/cache.go`

### 结果 A（热点 key：`Tom`）

- 总请求：`20000`
- 总耗时：`0.417s`
- QPS：`47946.61`
- 延迟：`avg 4.04ms / p50 3.02ms / p95 6.49ms / p99 50.56ms / max 91.43ms`

### 结果 B（混合 key：`Tom/Jack/Sam`）

- 总请求：`20000`
- 总耗时：`0.409s`
- QPS：`48889.58`
- 延迟：`avg 3.89ms / p50 2.62ms / p95 7.55ms / p99 49.93ms / max 119.05ms`

## 结论

- 分片的核心收益来自“多 key 并发分流”，不是单 key 魔法加速。
- 在本次口径下，混合 key 的 QPS 高于热点 key，符合分片预期。
- 若要进一步拉开优势，下一步应增加 key 基数（如 `1k+`）做更真实的分片对比。
