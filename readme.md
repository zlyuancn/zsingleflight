# zsingleflight

`zsingleflight` 是一个泛型实现的 Go 并发控制工具库，用于防止在高并发场景下对相同 key
的重复请求穿透到底层数据源（如数据库、远程接口等），有效缓解“缓存击穿”问题。

它确保：**对于同一个 key，在同一时刻只有一个 goroutine 执行实际的加载逻辑（`LoadInvoke`），其余并发请求将等待并复用该结果**。

---

## ✨ 特性

- **泛型支持**（Go 1.18+）：适用于任意返回类型。
- **分片锁（Sharded Locking）**：内部使用多个读写锁分片，减少锁竞争，提升并发性能。
- **轻量无依赖**：仅使用标准库，无第三方依赖。
- **安全并发**：正确处理竞态条件，保证**同一时刻**加载函数仅执行一次。
- **灵活分片数**：支持自定义分片数量（必须为 2 的幂）。

---

## 📦 安装

```bash
go get github.com/zlyuancn/zsingleflight
```

---

## 🚀 快速开始

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/zlyuancn/zsingleflight"
)

func main() {
	sf := zsingleflight.New[string]()

	// 模拟一个耗时的数据加载函数
	loadFromDB := func(ctx context.Context, key string) (string, error) {
		fmt.Printf("Loading data for key: %s\n", key)
		time.Sleep(100 * time.Millisecond) // 模拟 I/O 延迟
		return fmt.Sprintf("data_of_%s", key), nil
	}

	// 并发请求同一个 key
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := sf.Do(context.Background(), "user:123", loadFromDB)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			fmt.Println("Got:", result)
		}()
	}

	wg.Wait()
}
```

**输出示例**：

```
Loading data for key: user:123
Got: data_of_user:123
Got: data_of_user:123
Got: data_of_user:123
Got: data_of_user:123
Got: data_of_user:123
```

> 注意：`loadFromDB` 仅被调用一次，即使有 5 个并发请求。

---

## 🧠 使用建议

### ✅ 典型使用模式（配合缓存）

`zsingleflight` **不是缓存**，而是**防击穿协调器**。推荐与缓存层结合使用：

```go
func GetUserData(userID string) (User, error) {
// 1. 先查本地缓存（如带 TTL 的 memory cache）
if user, ok := localCache.Get(userID); ok {
return user, nil
}

// 2. 缓存未命中 → 使用 SingleFlight 防止多个 goroutine 同时回源
user, err := sf.Do(context.Background(), userID, func (ctx context.Context, key string) (User, error) {
// 从 DB 加载
u, err := db.GetUser(key)
if err == nil {
// 加载成功后写入缓存
localCache.Set(key, u, 5*time.Minute)
}
return u, err
})

return user, err
}
```

这样：

- 缓存命中 → 直接返回，不经过 `SingleFlight`
- 缓存未命中 → 多个并发请求只触发一次 DB 查询
- DB 查询完成后更新缓存，后续请求走缓存

---

## ⚙️ API 说明

### `New[T any](shardCount ...uint32) *SingleFlight[T]`

创建一个新的 `SingleFlight` 实例。

- `shardCount`（可选）：分片数量，必须是 2 的幂（如 64, 128, 256）。默认为 `256`（即 `1 << 8`）。
- 若传入非法值（非 2 的幂），会 panic

### `Do(ctx context.Context, key string, invoke LoadInvoke[T]) (T, error)`

执行加载逻辑。

- `key`：唯一标识符，相同 key 的并发请求会被合并。
- `invoke`：实际的数据加载函数，签名：`func(ctx context.Context, key string) (T, error)`
- 返回加载结果或错误。

**注意**：`ctx` 当前未在内部使用，仅作为 `LoadInvoke` 传参，若需支持超时/取消，请在 `LoadInvoke` 内部处理。

---

## 🧪 测试

项目包含完整单元测试，覆盖：

- 单次调用
- 高并发同 key 请求（验证仅执行一次）
- 错误传播
- 不同 key 隔离
- 自定义分片数

运行测试：

```bash
go test -v ./...
```

---

## 对比官方库

### ✅ 与官方 singleflight 的主要异同

| 维度               | 官方 `golang.org/x/sync/singleflight` | `zsingleflight`                     |
|------------------|-------------------------------------|-------------------------------------|
| **泛型支持**         | ❌                                   | ✅ 原生泛型 `T`                          |
| **分片（Sharding）** | ❌ 单一全局 map + mutex                  | ✅ 按 key hash 分片，降低锁粒度               |
| **性能**           | ❌单一锁导致竞争                            | ✅ 多分片, 减少锁竞争                        |
| **并发安全**         | ✅                                   | ✅                                   |
| **重复请求合并**       | ✅                                   | ✅                                   |
| **Context 支持**   | ✅（通过 `DoChan` 或闭包传入）                | ✅ 直接作为参数传入 `invoke`                 |
| **内存泄漏风险**       | ⚠️ 若 invoke 阻塞，map 中 entry 不释放      | ✅ 执行完后立即 `delete(wait, key)`，避免长期驻留 |
| **panic 安全性**    | ✅ recover 并传播 panic                 | ✅ recover 并传播 panic                 |

---

💡 **提示**：`zsingleflight` 适用于“瞬时并发抑制”，不适用于长期缓存，请务必配合 TTL 缓存使用。
