package limiter

import "golang.org/x/time/rate"

type MultiLimiter struct {
	limiters []*rate.Limiter
}

func NewMultiLimiter(l ...*rate.Limiter) *MultiLimiter {
	return &MultiLimiter{limiters: l}
}

func (ml *MultiLimiter) Allow() bool {
	for _, limiter := range ml.limiters {
		if !limiter.Allow() {
			return false
		}
	}
	return true
}