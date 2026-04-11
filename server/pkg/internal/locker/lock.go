package locker

import (
	"sync"
)

type refMutex struct {
	mu   sync.RWMutex
	refs int
}

// 定义分片锁结构体
type ShardedLock struct {
	mu    sync.Mutex
	locks sync.Map
}

func NewShardedLock() *ShardedLock {
	return &ShardedLock{
		locks: sync.Map{},
	}
}

// 根据key获取对应的锁
// 必须在 sl.mu 保护下完成 refs++，否则可能与 Unlock 的 refs--/Delete 产生竞态，
// 导致 get 返回的 rm 对应的 key 已被删除，后续 Unlock 时 Load(key) 返回 !ok 而 panic。
func (sl *ShardedLock) get(key string) *refMutex {
	sl.mu.Lock()
	v, loaded := sl.locks.Load(key)
	if loaded {
		rm := v.(*refMutex)
		rm.refs++
		sl.mu.Unlock()
		return rm
	}

	rm := &refMutex{refs: 1}
	v, loaded = sl.locks.LoadOrStore(key, rm)
	if loaded {
		real := v.(*refMutex)
		real.refs++
		sl.mu.Unlock()
		return real
	}
	sl.mu.Unlock()
	return rm
}

func (sl *ShardedLock) RLock(key string) {
	rm := sl.get(key)
	rm.mu.RLock()
}

func (sl *ShardedLock) RUnlock(key string) {
	v, ok := sl.locks.Load(key)
	if !ok {
		panic("unlock of unknown key")
	}
	rm := v.(*refMutex)
	rm.mu.RUnlock()

	sl.mu.Lock()
	rm.refs--
	if rm.refs == 0 {
		sl.locks.Delete(key)
	}
	sl.mu.Unlock()
}

func (sl *ShardedLock) Lock(key string) {
	rm := sl.get(key)
	rm.mu.Lock()
}

func (sl *ShardedLock) Unlock(key string) {
	v, ok := sl.locks.Load(key)
	if !ok {
		panic("unlock of unknown key")
	}
	rm := v.(*refMutex)
	rm.mu.Unlock()

	sl.mu.Lock()
	rm.refs--
	if rm.refs == 0 {
		sl.locks.Delete(key)
	}
	sl.mu.Unlock()
}

// 针对key执行加锁操作的示例方法
func (sl *ShardedLock) DoWithLock(key string, fn func()) {
	sl.Lock(key)
	defer sl.Unlock(key)
	fn() // 执行需要加锁的业务逻辑
}
