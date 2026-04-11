package resolver

import (
	"github.com/wangliang139/llt-trade/server/pkg/gateway/stream"
	"github.com/wangliang139/llt-trade/server/pkg/service/accountsvc"
	"github.com/wangliang139/llt-trade/server/pkg/service/alertsvc"
	"github.com/wangliang139/llt-trade/server/pkg/service/documentsvc"
	"github.com/wangliang139/llt-trade/server/pkg/service/llmsvc"
	"github.com/wangliang139/llt-trade/server/pkg/service/marketsvc"
	"github.com/wangliang139/llt-trade/server/pkg/service/ordersvc"
	"github.com/wangliang139/llt-trade/server/pkg/service/strategysvc"
	"github.com/wangliang139/llt-trade/server/pkg/service/streamsvc"
	"github.com/wangliang139/llt-trade/server/pkg/service/tgbotsvc"
	"github.com/wangliang139/llt-trade/server/pkg/service/usersvc"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

type Resolver struct {
	StrategySvc *strategysvc.Service
	UserSvc     *usersvc.Service
	StreamSvc   *streamsvc.Service
	AccountSvc  *accountsvc.Service
	OrderSvc    *ordersvc.Service
	DocumentSvc *documentsvc.Service
	LlmSvc      *llmsvc.Service
	MarketSvc   *marketsvc.Service
	TgBotSvc    *tgbotsvc.Service
	AlertSvc    *alertsvc.Service

	StreamManager *stream.Manager
}
