package converter

import (
	"strconv"

	"github.com/wangliang139/NovaForge/server/pkg/action/model"
	"github.com/wangliang139/NovaForge/server/pkg/service/usersvc"
	userapikey "github.com/wangliang139/NovaForge/server/pkg/repos/api_keys"
)

func UserAPIKeyToGql(row *userapikey.ApiKey) *model.UserAPIKey {
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