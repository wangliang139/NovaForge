package converter

import (
	"fmt"

	"github.com/bytedance/sonic"
	"github.com/wangliang139/llt-trade/server/pkg/repos/document"
	"github.com/wangliang139/llt-trade/server/pkg/repos/tg_channel"
	"github.com/wangliang139/llt-trade/server/pkg/types"
)

func ChannelRepo2Types(po *tg_channel.TgChannel) (*types.Channel, error) {
	if po == nil {
		return nil, nil
	}
	var extractCfg types.ExtractCfg
	if len(po.ExtractCfg) > 0 {
		if err := sonic.Unmarshal(po.ExtractCfg, &extractCfg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal extract cfg: %w", err)
		}
	}

	return &types.Channel{
		ID:         po.ID,
		Name:       po.Name,
		Title:      po.Title,
		Broadcast:  po.Broadcast,
		Source:     po.Source,
		Catalog:    document.DocumentCatalog(po.Catalog),
		ExtractCfg: extractCfg,
		Enabled:    po.Enabled,
		CreatedAt:  po.CreatedAt,
		UpdatedAt:  po.UpdatedAt,
	}, nil
}
