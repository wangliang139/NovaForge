package account

import (
	"github.com/wangliang139/NovaForge/server/pkg/repos/orders"
)

// classifyMultiBotOrderAccount 在已确认 stream 账户为「父 real + multi_bot」且各查询已执行的前提下，决定订单应写入的 account_id。
// 优先级：父 order_id → 子 order_id → 父 client_order_id → 父树下任意 client_order_id（含子）。
func classifyMultiBotOrderAccount(parentStreamID string,
	parentByOID, childByOID, parentByCID, underParentByCID *orders.Order,
) string {
	if parentByOID != nil {
		return parentStreamID
	}
	if childByOID != nil {
		return childByOID.AccountID
	}
	if parentByCID != nil {
		return parentStreamID
	}
	if underParentByCID != nil {
		return underParentByCID.AccountID
	}
	return parentStreamID
}
