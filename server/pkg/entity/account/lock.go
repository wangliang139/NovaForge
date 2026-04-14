package account

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/mow/errors"
)

// ctxKeyAccountWriteSkip 标记当前调用栈已持有账户写锁，避免可重入路径死锁。
type ctxKeyAccountWriteSkip struct{}

func WithAccountWriteSkip(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKeyAccountWriteSkip{}, true)
}

func AccountWriteSkipped(ctx context.Context) bool {
	v, _ := ctx.Value(ctxKeyAccountWriteSkip{}).(bool)
	return v
}

type accountWriteLocker struct {
	mu sync.Mutex
	m  map[string]*sync.Mutex
}

func (l *accountWriteLocker) mutexFor(id string) *sync.Mutex {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.m == nil {
		l.m = make(map[string]*sync.Mutex)
	}
	if l.m[id] == nil {
		l.m[id] = &sync.Mutex{}
	}
	return l.m[id]
}

func sortedUniqueAccountIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// WithSortedAccountWrites 按 account id 字典序依次加锁后执行 fn，用于避免多账户场景下死锁。
// 若 ctx 已带 WithAccountWriteSkip，则不再加锁，直接执行 fn。
func (e *Entity) WithSortedAccountWrites(ctx context.Context, accountIDs []string, fn func(context.Context) error) error {
	if AccountWriteSkipped(ctx) {
		return fn(ctx)
	}
	ids := sortedUniqueAccountIDs(accountIDs)
	if len(ids) == 0 {
		return fn(ctx)
	}
	mutexes := make([]*sync.Mutex, len(ids))
	for i, id := range ids {
		mutexes[i] = e.writeLocks.mutexFor(id)
	}
	for _, m := range mutexes {
		m.Lock()
	}
	defer func() {
		for i := len(mutexes) - 1; i >= 0; i-- {
			mutexes[i].Unlock()
		}
	}()
	return fn(ctx)
}

// AccountWriteLockIDsForTradingAccountChain 返回下单/刷新等写路径应对齐的互斥 id：
// virtual_sub 沿 parent 链直到非 virtual_sub，全部纳入并按字典序加锁。
func (e *Entity) AccountWriteLockIDsForTradingAccountChain(ctx context.Context, acct *types.Account) ([]string, error) {
	if acct == nil {
		return nil, errors.New(errors.InvalidArgument, "account is required")
	}
	var ids []string
	cur := acct
	for cur != nil && cur.AccountType == types.AccountTypeVirtualSub {
		ids = append(ids, strings.TrimSpace(cur.ID))
		if cur.ParentAccountID == nil || strings.TrimSpace(*cur.ParentAccountID) == "" {
			return nil, errors.New(errors.InvalidArgument, "virtual_sub account missing parent_account_id")
		}
		parent, err := e.GetAccount(ctx, strings.TrimSpace(*cur.ParentAccountID))
		if err != nil {
			return nil, err
		}
		if parent == nil {
			return nil, errors.New(errors.NotFound, "parent account not found")
		}
		if parent.Exchange != cur.Exchange {
			return nil, errors.New(errors.InvalidArgument, "virtual_sub parent exchange mismatch")
		}
		cur = parent
	}
	if cur != nil {
		ids = append(ids, strings.TrimSpace(cur.ID))
	}
	return sortedUniqueAccountIDs(ids), nil
}
