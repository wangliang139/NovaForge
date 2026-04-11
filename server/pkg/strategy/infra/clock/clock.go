package clock

import (
	"fmt"
	"time"
)

type Clock interface {
	Now() time.Time
}

type RealClock struct{}

var DefaultRealClock Clock = &RealClock{}

func (c *RealClock) Now() time.Time {
	return time.Now()
}

type BacktestClock struct {
	ts time.Time
}

var _ Clock = &BacktestClock{}

func (c *BacktestClock) Now() time.Time {
	return c.ts
}

func (c *BacktestClock) Set(ts time.Time) error {
	if ts.Before(c.ts) {
		return fmt.Errorf("clock set to past time: %s < %s", ts.Format(time.RFC3339), c.ts.Format(time.RFC3339))
	}
	c.ts = ts
	return nil
}

func NewBacktestClock(startTs time.Time) *BacktestClock {
	return &BacktestClock{
		ts: startTs,
	}
}
