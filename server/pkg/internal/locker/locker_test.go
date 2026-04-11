package locker

import (
	"fmt"
	"sync"
	"testing"
)

// 测试示例
func TestLocker(t *testing.T) {
	shardLock := NewShardedLock()
	wg := sync.WaitGroup{}

	var data sync.Map

	// 并发对3个key进行累加操作
	keys := []string{"key1", "key2", "key3"}
	for i := 0; i < 1000; i++ {
		for _, key := range keys {
			wg.Add(1)
			go func(k string) {
				defer wg.Done()
				shardLock.DoWithLock(k, func() {
					val, ok := data.Load(k)
					if !ok {
						val = 0
					}
					data.Store(k, val.(int)+1)
				})
			}(key)
		}
	}

	wg.Wait()
	// 输出结果：每个key的值应为1000
	for _, key := range keys {
		val, ok := data.Load(key)
		if !ok {
			val = 0
		}
		fmt.Printf("%s: %d\n", key, val.(int))
	}
}
