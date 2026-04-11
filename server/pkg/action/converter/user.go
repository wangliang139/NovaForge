package converter

import (
	"strconv"

	"github.com/wangliang139/llt-trade/server/pkg/action/model"
	"github.com/wangliang139/llt-trade/server/pkg/service/usersvc"
	userapikey "github.com/wangliang139/llt-trade/server/pkg/repos/user_api_key"
)

func UserAPIKeyToGql(row *userapikey.UserApiKey) *model.UserAPIKey {
	if row == nil {
		return nil
	}
	perms := make([]model.UserAPIKeyPermission, 0, len(row.Permissions))
	for _, p := range row.Permissions {
		switch p {
		case usersvc.APIKeyPermissionQuery:
			perms = append(perms, model.UserAPIKeyPermissionQuery)
		case usersvc.APIKeyPermissionTrade:
			perms = append(perms, model.UserAPIKeyPermissionTrade)
		}
	}
	return &model.UserAPIKey{
		ID:          strconv.FormatInt(row.ID, 10),
		Name:        row.Name,
		KeyPrefix:   row.KeyPrefix,
		Permissions: perms,
		CreatedAt:   int(row.CreatedAt.UTC().UnixMilli()),
	}
}