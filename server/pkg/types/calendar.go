package types

import (
	"time"

	"github.com/wangliang139/llt-trade/server/pkg/repos/calendar"
)

type Calendar struct {
	ID          int64
	DateID      int32
	Sid         string
	Source      calendar.CalendarSource
	Type        calendar.CalendarType
	Category    string
	Country     *string
	Project     *string
	Symbol      *string
	Title       string
	Content     string
	Importance  int32
	Url         string
	Ext         []byte
	PublishedAt time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type EconomicCalendarExtension struct {
	Unit      string `json:"unit"`
	Actual    string `json:"actual"`
	Previous  string `json:"previous"`
	Consensus string `json:"consensus"`
	Revised   string `json:"revised"`
}

type QueryCalendarInput struct {
	DateID        int32
	Source        *calendar.CalendarSource
	Type          *calendar.CalendarType
	Category      *string
	Country       *string
	MinImportance *int32
}
