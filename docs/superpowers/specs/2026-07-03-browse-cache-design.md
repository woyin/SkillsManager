# skills.sh browse 本地缓存

**日期**：2026-07-03
**范围**：性能优化，browse 命令

## 背景

`sm browse` 是交互式浏览 skills.sh 目录，用户常反复运行。当前每次都打 `https://skills.sh/api/v1` 实时请求（15s 超时），无缓存：慢、离线不可用、重复查询浪费带宽。

## 目标

文件缓存 + TTL：命中且未过期则用缓存，否则请求网络并写回。`--no-cache` 绕过，`--refresh` 强制刷新。

## 设计

### 缓存注入点

所有 API 调用都经过 `fetchSkillsAPI(endpoint, token)`。在它内部加缓存层：缓存 key = `endpoint`（含 query string，已区分不同查询/agent/topic）。

### 缓存存储

- 位置：`<DataDir>/cache/browse/`
- 文件名：endpoint 的 SHA-256 hex（key 含 `/`、`?` 不宜直接做文件名）
- 内容：原始 JSON body 字节（不经反序列化，直接落盘 + 读回复用，零解析损耗）
- 元数据：存一个伴随 `.meta` 文件记录 fetch 时刻（Unix 秒），或用文件 mtime 判过期。**用 mtime**（零额外文件，`os.Stat` 即得）。

### TTL

默认 10 分钟（`browseCacheTTL = 10 * time.Minute`）。常理：skills.sh 目录更新频率不高，10 分钟内重复 browse 体验一致且数据足够新。

### 旁路控制

- `--no-cache`：跳过读缓存，但仍写（让下次受益）。
- `--refresh`：跳过读缓存（强制网络），写新结果。
- 二者效果相近；保留两个语义清晰：`--no-cache` 一次性诊断，`--refresh` 主动拉新。为简化，**只实现 `--refresh`**（YAGNI：`--no-cache` 与 `--refresh` 行为几乎相同，留一个）。

### 流程

```
fetchSkillsAPI(endpoint, token):
  if not refresh:
    if cached := readCache(key); cached != nil && fresh(cached, TTL):
      return parse(cached)   // 命中
  body, err := httpGet(...)
  if err != nil:
    // 网络失败时，若有过期缓存，降级用它（离线友好）
    if cached := readCache(key); cached != nil:
      return parse(cached)
    return err
  writeCache(key, body)
  return parse(body)
```

**离线降级**：网络失败时用过期缓存，browse 离线仍可用（数据可能旧但优于报错）。

### 隔离性

- 缓存层纯函数式：读 key、读/写文件、返回字节。
- 不影响 `parseSkillsResponse` 等解析逻辑。
- token 不进缓存 key（同一 endpoint 不同 token 缓存共享——acceptable：token 影响鉴权而非公开目录数据）。

## 验证

1. `cmd/browse_test.go` 新增：
   - 命中：第二次同 endpoint 用缓存（用 fake httptest server 计数请求次数）
   - TTL 过期：调 mtime 到过期后 → 重新请求
   - 离线降级：网络 endpoint 失败 + 过期缓存存在 → 返回缓存
2. 手测：`sm browse <query>` 两次，第二次明显更快；断网后仍能 browse（缓存命中）

## 非目标

- 不缓存无 token 的 website scrape 路径（仅 API 缓存）
- 不做缓存大小限制 / LRU（browse 查询量小，目录自然增长缓慢；超时手动清 `rm <DataDir>/cache`）
- 不缓存分页全量（按 endpoint key 自然分页各自缓存）
