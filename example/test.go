package main

import (
	"context"
	"fmt"
	"sync"
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
