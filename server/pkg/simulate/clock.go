package simulate

import "time"

// ReplayClock is a deterministic clock for market replay.
type ReplayClock struct {
	now time.Time
}

// NewReplayClock creates a clock pinned at start.
func NewReplayClock(start time.Time) *ReplayClock {
	return &ReplayClock{now: start.UTC()}
}

// Now returns current replay time.
func (c *ReplayClock) Now() time.Time {
	if c == nil {
		return time.Time{}
	}
	return c.now
}

// AdvanceTo moves time forward only.
func (c *ReplayClock) AdvanceTo(ts time.Time) {
	if c == nil {
		return
	}
	if ts.After(c.now) {
		c.now = ts.UTC()
	}
}

// AdvanceBy moves time by duration.
func (c *ReplayClock) AdvanceBy(d time.Duration) {
	if c == nil || d <= 0 {
		return
	}
	c.now = c.now.Add(d)
}
