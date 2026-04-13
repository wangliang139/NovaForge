package account

import (
	"testing"

	"github.com/wangliang139/NovaForge/server/pkg/repos/orders"
)

func TestClassifyMultiBotOrderAccount(t *testing.T) {
	const parent = "parent-1"
	const child = "child-1"

	parentRow := &orders.Order{AccountID: parent}
	childRow := &orders.Order{AccountID: child}

	tests := []struct {
		name                          string
		parentByOID, childByOID       *orders.Order
		parentByCID, underParentByCID *orders.Order
		want                          string
	}{
		{
			name:       "parent_hit_by_order_id",
			parentByOID: parentRow,
			want:        parent,
		},
		{
			name:       "child_hit_by_order_id",
			childByOID: childRow,
			want:       child,
		},
		{
			name:        "parent_hit_by_client_id",
			parentByCID: parentRow,
			want:        parent,
		},
		{
			name:             "child_hit_by_client_under_parent",
			underParentByCID: childRow,
			want:             child,
		},
		{
			name:             "parent_precedence_over_under_parent_client",
			parentByOID:      parentRow,
			underParentByCID: childRow,
			want:             parent,
		},
		{
			name:       "child_order_id_before_parent_client",
			childByOID: childRow,
			parentByCID: parentRow,
			want:       child,
		},
		{
			name: "miss_falls_back_to_parent_stream",
			want: parent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyMultiBotOrderAccount(parent, tt.parentByOID, tt.childByOID, tt.parentByCID, tt.underParentByCID)
			if got != tt.want {
				t.Fatalf("classifyMultiBotOrderAccount(...) = %q, want %q", got, tt.want)
			}
		})
	}
}
