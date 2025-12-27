/*
-------------------------------------------------
   Author :       zlyuan
   date：         2019/12/13
   Description :
-------------------------------------------------
*/

package zsingleflight

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDo(t *testing.T) {
	var sf = New[string]()

	var dv = "value"

	c := make(chan string)
	var calls int32
	fn := func(ctx context.Context, key string) (string, error) {
		atomic.AddInt32(&calls, 1)
		return <-c, nil
	}

	const n = 100000 // 模拟并发请求次数
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			v, err := sf.Do(context.Background(), "key", fn)
			if err != nil {
				t.Errorf("Do函数执行失败: %v", err)
			}
			if v != dv {
				t.Errorf("收到值 %q; 它应该是 %q", v, dv)
			}
		}()
	}

	time.Sleep(time.Second) // 等待所有协程都拿到了 waitResult.wg
	c <- dv                 // 发送数据让加载器执行
	wg.Wait()               // 等待所有goroutinue执行完毕

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("调用次数 = %d; 它应该是 1", got)
	}
}

func Benchmark_Do(b *testing.B) {
	var sf = New[int]()

	b.ResetTimer()

	b.RunParallel(func(p *testing.PB) {
		i := 0
		for p.Next() {
			sr := new(big.Int).SetInt64(10000)
			n, _ := rand.Int(rand.Reader, sr)
			m := n.Int64()

			_, _ = sf.Do(context.Background(), strconv.Itoa(int(m)), func(ctx context.Context, key string) (int, error) {
				return 0, nil
			})
			i++
		}
	})
}

func TestSingleFlight_Do(t *testing.T) {
	sf := New[int]()

	key := "test-key"
	expected := 42

	result, err := sf.Do(context.Background(), key, func(ctx context.Context, k string) (int, error) {
		if k != key {
			t.Errorf("expected key %s, got %s", key, k)
		}
		return expected, nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != expected {
		t.Errorf("expected %d, got %d", expected, result)
	}
}

func TestSingleFlight_DifferentKeys(t *testing.T) {
	sf := New[int]()
	keys := []string{"key1", "key2", "key3"}
	expected := map[string]int{"key1": 1, "key2": 2, "key3": 3}
	var mu sync.Mutex
	callLog := make(map[string]int)

	invoke := func(ctx context.Context, k string) (int, error) {
		mu.Lock()
		callLog[k]++
		mu.Unlock()
		return expected[k], nil
	}

	var wg sync.WaitGroup
	for _, k := range keys {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			res, err := sf.Do(context.Background(), key, invoke)
			if err != nil {
				t.Errorf("unexpected error for key %s: %v", key, err)
			}
			if res != expected[key] {
				t.Errorf("for key %s: expected %d, got %d", key, expected[key], res)
			}
		}(k)
	}

	wg.Wait()

	// 每个 key 应该只调用一次
	for _, k := range keys {
		if callLog[k] != 1 {
			t.Errorf("key %s called %d times, expected 1", k, callLog[k])
		}
	}
}

func TestSingleFlight_CustomShardCount(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()

	// 正确：2 的幂
	sf := New[int](16)
	if len(sf.mxs) != 16 {
		t.Errorf("expected 16 shards, got %d", len(sf.mxs))
	}

	// 测试功能是否正常
	res, err := sf.Do(context.Background(), "test", func(ctx context.Context, key string) (int, error) {
		return 999, nil
	})
	if err != nil || res != 999 {
		t.Errorf("custom shard Do failed: res=%d err=%v", res, err)
	}
}

func TestSingleFlight_ErrorPropagation(t *testing.T) {
	sf := New[int]()
	key := "error-key"
	expectedErr := errors.New("simulated error")
	var callCount int32

	invoke := func(ctx context.Context, k string) (int, error) {
		atomic.AddInt32(&callCount, 1)
		time.Sleep(time.Second) // 等待所有协程都能拿到 waitResult.wg
		return 0, expectedErr
	}

	const goroutines = 5
	var wg sync.WaitGroup
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := sf.Do(context.Background(), key, invoke)
			errs[idx] = err
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if !errors.Is(err, expectedErr) {
			t.Errorf("goroutine %d: expected error %v, got %v", i, expectedErr, err)
		}
	}

	if actual := atomic.LoadInt32(&callCount); actual != 1 {
		t.Errorf("expected invoke to be called once, but called %d times", actual)
	}
}

func TestSingleFlight_SharedCountNotPowerOfTwo(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for non-power-of-two shard count")
		}
	}()

	_ = New[int](10) // 10 不是 2 的幂
}
