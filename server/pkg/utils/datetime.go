package utils

import "time"

type _datetime struct{}

var Datetime = _datetime{}

func (d _datetime) UnixToDateID(unix int64) int32 {
	t := time.Unix(unix, 0)
	return d.TimeToDateID(t)
}

func (d _datetime) TimeToDateID(t time.Time) int32 {
	t = t.In(time.UTC)
	return int32(t.Year()*10000 + int(t.Month())*100 + t.Day())
}