package types

import (
	"time"
)

type DataSource struct {
	ID          int32
	Type        SignalType
	Name        string
	Description string
	Exchange    *Exchange
	Symbol      *Symbol
	Props       map[string]any
	StartTs     time.Time
	EndTs       time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CreateDatasourceInput 用于创建K线数据源并回填 ds_items。
// 仅包含本期需要的字段；后续扩展其它 datasource 类型时再补齐。
type CreateDatasourceInput struct {
	Type        SignalType
	Name        string
	Description string
	Exchange    *Exchange
	Symbol      *Symbol
	Props       map[string]any
	StartTs     time.Time
	EndTs       time.Time
}

// DatasourceFilter 数据源过滤条件
type DatasourceFilter struct {
	ID       *int32
	Type     *SignalType
	Exchange *Exchange
	Symbol   *Symbol
	Offset   int64
	Limit    int64
}
