/*
-------------------------------------------------
   Author :       zlyuan
   date：         2019/12/13
   Description :  用于解决缓存击穿的单飞模块
-------------------------------------------------
*/

package zsingleflight

import "sync"

type call struct {
    wg sync.WaitGroup
    v  interface{}
    e  error
}

type singleFlight struct {
    mu sync.Mutex
    m  map[string]*call
}

func New() *singleFlight {
    return &singleFlight{
        m: make(map[string]*call),
    }
}

func (m *singleFlight) Do(key string, fn func() (interface{}, error)) (interface{}, error) {
    m.mu.Lock()

    // 来晚了, 等待结果
    if c, ok := m.m[key]; ok {
        m.mu.Unlock()
        c.wg.Wait()
        return c.v, c.e
    }

    // 占位置
    c := new(call)
    c.wg.Add(1)
    m.m[key] = c
    m.mu.Unlock()

    // 执行db加载
    c.v, c.e = fn()
    c.wg.Done()

    // 离开
    m.mu.Lock()
    delete(m.m, key)
    m.mu.Unlock()

    return c.v, c.e
}
