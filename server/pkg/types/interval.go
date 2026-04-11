package types

import (
	"fmt"
	"time"
)

type Interval string

const (
	Interval1s  Interval = "1s"
	Interval1m  Interval = "1m"
	Interval5m  Interval = "5m"
	Interval15m Interval = "15m"
	Interval30m Interval = "30m"
	Interval1h  Interval = "1h"
	Interval4h  Interval = "4h"
	Interval12h Interval = "12h"
	Interval1d  Interval = "1d"
	Interval3d  Interval = "3d"
	Interval1w  Interval = "1w"
	Interval1M  Interval = "1M"
)

func (i Interval) String() string {
	return string(i)
}

func (i Interval) Valid() bool {
	switch i {
	case Interval1s, Interval1m, Interval5m, Interval15m, Interval30m, Interval1h, Interval4h, Interval12h, Interval1d, Interval3d, Interval1w, Interval1M:
		return true
	}
	return false
}

func Intervals() []Interval {
	return []Interval{Interval1s, Interval1m, Interval5m, Interval15m, Interval30m, Interval1h, Interval4h, Interval12h, Interval1d, Interval3d, Interval1w, Interval1M}
}

func (i Interval) Duration() (time.Duration, error) {
	if !i.Valid() {
		return 0, fmt.Errorf("invalid interval: %s", i)
	}
	switch i {
	case Interval1s:
		return 1 * time.Second, nil
	case Interval1m:
		return 1 * time.Minute, nil
	case Interval5m:
		return 5 * time.Minute, nil
	case Interval15m:
		return 15 * time.Minute, nil
	case Interval30m:
		return 30 * time.Minute, nil
	case Interval1h:
		return 1 * time.Hour, nil
	case Interval4h:
		return 4 * time.Hour, nil
	case Interval12h:
		return 12 * time.Hour, nil
	case Interval1d:
		return 24 * time.Hour, nil
	case Interval3d:
		return 3 * 24 * time.Hour, nil
	case Interval1w:
		return 7 * 24 * time.Hour, nil
	case Interval1M:
		return 30 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported interval: %s", i)
	}
}
