package gateio

type Response[T any] struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
	Data      T      `json:"data"`
}

type PageInfo struct {
	Total       int `json:"total"`
	CurrentPage int `json:"currentPage"`
	TotalPage   int `json:"totalPage"`
}

type ListFutureResponse struct {
	PageInfo PageInfo  `json:"pageInfo"`
	Items    []*Future `json:"items"`
}

type Future struct {
	ID               int            `json:"id"`
	Category         string         `json:"category"`
	SecondCategory   string         `json:"second_category"`
	IsImportantEvent string         `json:"is_important_event"`
	Symbol           *string        `json:"symbol,omitempty"`
	PubTime          int64          `json:"pub_time"`
	Country          *string        `json:"country,omitempty"`
	ProjectName      *string        `json:"project_name,omitempty"`
	ProjectIcon      *string        `json:"project_icon,omitempty"`
	ViewCount        int            `json:"view_count"`
	LikeCount        int            `json:"like_count"`
	ExtensionInfo    *ExtensionInfo `json:"extension_info,omitempty"`
	Title            string         `json:"title"`
	ContentText      string         `json:"content_text"`
	IconUrl          *string        `json:"icon_url,omitempty"`
	SymbolPercent    *string        `json:"symbol_percent,omitempty"`
	TargetUrl        string         `json:"target_url"`
}

type ExtensionInfo struct {
	Unit      string `json:"unit"`
	Actual    string `json:"actual"`
	Previous  string `json:"previous"`
	Consensus string `json:"consensus"`
}
