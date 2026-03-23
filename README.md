# GeeCache

分布式缓存 GeeCache 学习项目。

## 性能目标

- 目标区间：`100k ~ 200k QPS`（对齐 Redis 常见读场景量级）
- 当前策略：小步快跑，逐轮优化并固定口径压测

## 压测口径

- 接口：`http://localhost:9999/api?key=<key>`
- 节点：`8001/8002/8003`（`8003` 同时开启 API）
- 并发：`200`
- 请求数：`10000` 或 `20000`
- 机器：Windows / AMD Ryzen 7 7735H

## 第一轮（原始版本）

- 总请求：`10000`
- 总耗时：`0.362s`
- QPS：`27636.38`
- 延迟：`avg 7.16ms / p50 5.56ms / p95 11.16ms / p99 63.32ms / max 82.64ms`

## 第二轮（基础性能优化）

### 优化点

- 去掉缓存命中高频日志（`geecache/geecache.go`）
- `PickPeer` 改读锁（`geecache/http.go`）
- 复用 `http.Client` + KeepAlive（`geecache/http.go`）
- 修复远程 URL 拼接与错误返回（`geecache/http.go`）
- 增加并发基准测试（`geecache/benchmark_test.go`）

### 结果

- 总请求：`10000`
- 总耗时：`0.300s`
- QPS：`33283.64`
- 延迟：`avg 5.61ms / p50 3.78ms / p95 8.72ms / p99 60.39ms / max 101.42ms`

> 对比第一轮：QPS 约 `+20.4%`，avg/p95 约 `-21%`。

## 第三轮（LRU 分片）

### 优化点

- 本地缓存改为 `16` 分片（每片独立 `LRU + Mutex`）：`geecache/cache.go`
- 总容量按分片均分：`geecache/cache.go`
- `key -> shard` 使用无分配 FNV 哈希：`geecache/cache.go`

### 结果

- 热点 key（`Tom`，`20000` 请求）：`QPS 47946.61`，`avg 4.04ms`
- 混合 key（`Tom/Jack/Sam`，`20000` 请求）：`QPS 48889.58`，`avg 3.89ms`

## 第四轮（并发与分配优化）

### 优化点

- `sync.Pool` 复用 `pb.Request/pb.Response`，减少临时对象分配（`geecache/geecache.go`）
- `HTTPPool` 日志默认关闭，避免热路径日志 I/O（`geecache/http.go`）
- 新增 `ByteView.Bytes()`，响应写回路径减少一次拷贝（`geecache/byteview.go`, `geecache/http.go`, `main.go`）

### 两次复测结果（`20000` 请求，并发 `200`）

- 热点 key（`Tom`）
  - 第 1 次：`QPS 54871.10`，`avg 3.48ms / p95 5.73ms`
  - 第 2 次：`QPS 52241.27`，`avg 3.68ms / p95 6.09ms`
- 混合 key（`Tom/Jack/Sam`）
  - 第 1 次：`QPS 37936.34`，`avg 5.10ms / p95 10.39ms`
  - 第 2 次：`QPS 42594.41`，`avg 4.52ms / p95 9.02ms`

## 第五轮（协议精简 + 远端结果本地回填）

### 优化点

- Peer 接口改为直接传 `group/key`，返回原始 `[]byte`，移除 Protobuf 编解码链路（`geecache/peers.go`, `geecache/geecache.go`, `geecache/http.go`）
- 远端拉取成功后立刻回填本地缓存，避免同 key 重复跨节点（`geecache/geecache.go`）

### 结果（`20000` 请求，并发 `200`）

- 热点 key（`Tom`）：`QPS 54352.04`，`avg 3.49ms / p95 9.75ms`
- 混合 key（`Tom/Jack/Sam`）：`QPS 44419.84`，`avg 4.26ms / p95 11.92ms`

## 第六轮（net/http → fasthttp 迁移）

### 优化点

- 服务端 handler 从 `ServeHTTP(w, r)` 替换为 `HandleRequest(ctx *fasthttp.RequestCtx)`（`geecache/http.go`）
- 客户端 getter 从 `http.Client` 替换为 `fasthttp.Client`，使用 `AcquireRequest/Response` 对象池（`geecache/http.go`）
- 每个 peer 独立 `fasthttp.Client{MaxConnsPerHost: 64}`（`geecache/http.go`）
- `main.go` 两个启动函数适配 fasthttp API（`main.go`）

### 结果（`20000` 请求，并发 `200`）

- 热点 key（`Tom`）
  - 第 1 次：`QPS 63150.55`，`avg 2.80ms / p95 5.00ms`
  - 第 2 次：`QPS 67199.11`，`avg 2.60ms / p95 6.50ms`
- 混合 key（`Tom/Jack/Sam`，`60000` 请求）：`QPS 65280.71`，`avg 2.70ms / p95 5.13ms`

> 对比第五轮：热点 key QPS 从 `~55k` → `~67k`（`+22%`），avg 延迟从 `3.49ms` → `2.60ms`（`-25%`）。

## 第七轮（HTTP → 自定义二进制 TCP 协议）

### 优化点

- Peer 间通信从 HTTP（fasthttp）替换为自定义二进制 TCP 协议（`geecache/tcp.go`）
- 请求帧头从 ~200-500 字节 HTTP 头降至 4 字节：`[uint16 groupLen][uint16 keyLen][group][key]`
- 响应帧头从 ~200 字节 HTTP 头降至 5 字节：`[uint8 status][uint32 bodyLen][body]`
- 栈式连接池（上限 64），keep-alive 复用连接，溢出不阻塞
- 删除 `geecache/http.go`，`main.go` 适配 `TCPPool`

### 结果（`20000` 请求，并发 `200`）

- 热点 key（`Tom`）
  - 第 1 次：`QPS 70543.15`，`avg 2.50ms / p95 4.70ms`
  - 第 2 次：`QPS 66049.94`，`avg 2.70ms / p95 5.50ms`
- 混合 key（`Tom/Jack/Sam`，`60000` 请求）：`QPS 88666.93`，`avg 2.13ms / p95 4.87ms`

> 对比第六轮：热点 key QPS 从 `~67k` → `~70k`（`+5%`），混合 key QPS 从 `~65k` → `~89k`（`+36%`），avg 延迟从 `2.70ms` → `2.13ms`（`-21%`）。

## 阶段结论

- 当前最好成绩：热点 key 约 `70k QPS`，混合 key 约 `89k QPS`，较第一轮（`27.6k`）提升约 `2.5x ~ 3.2x`。
- 二进制 TCP 协议对混合 key 场景提升显著（`+36%`），热点 key 小幅提升（`+5%`，因本地缓存命中率高，peer 通信占比小）。
- 距离 `100k+ QPS` 目标已非常接近，下一步建议：
  - 扩大 key 基数（如 `1k+`）做更真实分片收益评估
  - 增加批量请求（batch get）减少 syscall 次数
  - 引入更贴近生产的压测工具与 CPU/GC 指标采集
