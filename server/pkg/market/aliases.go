package market

import (
	"github.com/wangliang139/llt-trade/server/pkg/market/types"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
)

type (
	Connector      = types.Connector
	Message        = ctypes.Message
	StreamSelector = ctypes.StreamSelector
	Subscription   = ctypes.Subscription
	StreamType     = ctypes.StreamType
	ConnectMode    = types.ConnectMode
	StreamHandle   = types.StreamHandle
	ApiAccount     = types.ApiAccount
)

var (
	TopicName           = ctypes.TopicName
	StreamKey           = ctypes.StreamKey
	NewSecretApiAccount = types.NewSecretApiAccount
	NewPlainApiAccount  = types.NewPlainApiAccount
)
