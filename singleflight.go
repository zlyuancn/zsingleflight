/*
-------------------------------------------------
   Author :       zlyuan
   date：         2019/12/13
   Description :  用于解决缓存击穿的单飞模块
-------------------------------------------------
*/

package zsingleflight

import (
    "hash/fnv"
    "sync"
)

type call struct {
    wg sync.WaitGroup
    v  interface{}
    e  error
}

type SingleFlight struct {
    mxs   []*sync.Mutex
    mms   []map[uint64]*call
    shard uint64
    mod_v uint64
}

func New(shard ...uint64) *SingleFlight {
    shard_count := uint64(16)
    if len(shard) > 0 {
        shard_count = shard[0]
    }
    return NewWithShard(shard_count)
}

func NewWithShard(shard uint64) *SingleFlight {
    if shard == 0 {
        shard = 16
    }

    shard_num := uint64(2)
    for shard_num < shard {
        shard_num *= 2
    }

    mxs := make([]*sync.Mutex, shard_num)
    mms := make([]map[uint64]*call, shard_num)
    for i := uint64(0); i < shard_num; i++ {
        mxs[i] = new(sync.Mutex)
        mms[i] = make(map[uint64]*call)
    }
    return &SingleFlight{
        mxs:   mxs,
        mms:   mms,
        shard: shard_num,
        mod_v: shard_num - 1,
    }
}

func fnv64a(key string) uint64 {
    m := fnv.New64a()
    _, _ = m.Write([]byte(key))
    return m.Sum64()
}

func (m *SingleFlight) Do(key string, fn func() (interface{}, error)) (interface{}, error) {
    hash := fnv64a(key)
    return m.DoHashKey(hash, fn)
}

func (m *SingleFlight) DoHashKey(hash uint64, fn func() (interface{}, error)) (interface{}, error) {
    shard := hash & m.mod_v
    mx := m.mxs[shard]
    mm := m.mms[shard]

    mx.Lock()

    // 来晚了, 等待结果
    if c, ok := mm[hash]; ok {
        mx.Unlock()
        c.wg.Wait()
        return c.v, c.e
    }

    // 占位置
    c := new(call)
    c.wg.Add(1)
    mm[hash] = c
    mx.Unlock()

    // 执行db加载
    c.v, c.e = fn()
    c.wg.Done()

    // 离开
    mx.Lock()
    delete(mm, hash)
    mx.Unlock()

    return c.v, c.e
}
