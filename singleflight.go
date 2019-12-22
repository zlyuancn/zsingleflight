/*
-------------------------------------------------
   Author :       zlyuan
   date：         2019/12/13
   Description :  用于解决缓存击穿的单飞模块
-------------------------------------------------
*/

package zsingleflight

import (
    "hash/crc32"
    "sync"
)

type call struct {
    wg sync.WaitGroup
    v  interface{}
    e  error
}

type SingleFlight struct {
    mxs   []*sync.Mutex
    mms   []map[string]*call
    shard uint32
}

func New() *SingleFlight {
    return NewWithShard(0)
}

// 指定分片
func NewWithShard(shard uint32) *SingleFlight {
    if shard <= 0 {
        shard = 10
    }

    mxs := make([]*sync.Mutex, shard)
    mms := make([]map[string]*call, shard)
    for i := uint32(0); i < shard; i++ {
        mxs[i] = new(sync.Mutex)
        mms[i] = make(map[string]*call)
    }
    return &SingleFlight{
        mxs:   mxs,
        mms:   mms,
        shard: shard,
    }
}

func (m *SingleFlight) Do(key string, fn func() (interface{}, error)) (interface{}, error) {
    shard := crc32.ChecksumIEEE([]byte(key)) % m.shard
    mx := m.mxs[shard]
    mm := m.mms[shard]

    mx.Lock()

    // 来晚了, 等待结果
    if c, ok := mm[key]; ok {
        mx.Unlock()
        c.wg.Wait()
        return c.v, c.e
    }

    // 占位置
    c := new(call)
    c.wg.Add(1)
    mm[key] = c
    mx.Unlock()

    // 执行db加载
    c.v, c.e = fn()
    c.wg.Done()

    // 离开
    mx.Lock()
    delete(mm, key)
    mx.Unlock()

    return c.v, c.e
}
