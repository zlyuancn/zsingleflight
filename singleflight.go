package zsingleflight

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"sync"
)

type LoadInvoke[T any] func(ctx context.Context, key string) (T, error)

// 单跑接口
type ISingleFlight[T any] interface {
	// 执行, 当缓存数据库不存在时, 在执行loader加载数据前, 会调用此方法
	Do(ctx context.Context, key string, invoke LoadInvoke[T]) (T, error)
}

const (
	ShardCount uint32 = 1 << 8 // 分片数
)

type waitResult[T any] struct {
	wg sync.WaitGroup
	v  T
	e  error
}

type SingleFlight[T any] struct {
	mxs           []*sync.RWMutex
	waits         []map[string]*waitResult[T]
	shardAndOPVal uint32 // 按位与操作值
}

// 创建一个 SingleFlight. shard 必须是 2 的幂次方
func New[T any](shardCount ...uint32) *SingleFlight[T] {
	count := ShardCount
	if len(shardCount) > 0 && shardCount[0] > 0 {
		count = shardCount[0]
		if count&(count-1) != 0 {
			panic(errors.New("shardCount must power of 2"))
		}
	}

	mxs := make([]*sync.RWMutex, count)
	mms := make([]map[string]*waitResult[T], count)
	for i := uint32(0); i < count; i++ {
		mxs[i] = new(sync.RWMutex)
		mms[i] = make(map[string]*waitResult[T])
	}
	return &SingleFlight[T]{
		mxs:           mxs,
		waits:         mms,
		shardAndOPVal: count - 1,
	}
}

func (m *SingleFlight[T]) getShard(key string) (*sync.RWMutex, map[string]*waitResult[T]) {
	f := fnv.New32a()
	_, _ = f.Write([]byte(key))
	n := f.Sum32()
	shard := n & m.shardAndOPVal
	return m.mxs[shard], m.waits[shard]
}

func (m *SingleFlight[T]) Do(ctx context.Context, key string, invoke LoadInvoke[T]) (T, error) {
	mx, wait := m.getShard(key)

	mx.RLock()
	result, ok := wait[key]
	mx.RUnlock()

	// 已经有线程在查询
	if ok {
		result.wg.Wait()
		return result.v, result.e
	}

	mx.Lock()

	// 再检查一下, 因为在拿到锁之前可能被别的线程占了位置
	result, ok = wait[key]
	if ok {
		mx.Unlock()
		result.wg.Wait()
		return result.v, result.e
	}

	// 占位置
	result = new(waitResult[T])
	result.wg.Add(1)
	wait[key] = result
	mx.Unlock()

	// 执行db加载
	result.v, result.e = safeInvoke(invoke, ctx, key)
	result.wg.Done()

	// 删除位置
	mx.Lock()
	delete(wait, key)
	mx.Unlock()

	return result.v, result.e
}

func safeInvoke[T any](f LoadInvoke[T], ctx context.Context, key string) (v T, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("invoke panicked: %v", r)
		}
	}()
	return f(ctx, key)
}
