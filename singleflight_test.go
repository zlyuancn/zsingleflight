/*
-------------------------------------------------
   Author :       zlyuan
   date：         2019/12/13
   Description :
-------------------------------------------------
*/

package zsingleflight

import (
    "sync"
    "sync/atomic"
    "testing"
)

func TestDo(t *testing.T) {
    var sf = New()

    var dv = "value"

    c := make(chan string)
    var calls int32
    fn := func() (interface{}, error) {
        atomic.AddInt32(&calls, 1)
        return <-c, nil
    }

    const n = 100000 // 模拟并发请求次数
    var wg sync.WaitGroup
    var wg2 sync.WaitGroup
    for i := 0; i < n; i++ {
        wg.Add(1)
        wg2.Add(1)
        go func() {
            wg.Done()
            v, err := sf.Do("key", fn)
            if err != nil {
                t.Errorf("Do函数执行失败: %v", err)
            }
            if v.(string) != dv {
                t.Errorf("收到值 %q; 它应该是 %q", v, dv)
            }
            wg2.Done()
        }()
    }

    wg.Wait() // 等待所有goroutinue就绪
    c <- dv
    wg2.Wait() // 等待所有goroutinue执行完毕

    if got := atomic.LoadInt32(&calls); got != 1 {
        t.Errorf("调用次数 = %d; 它应该是 1", got)
    }
}
