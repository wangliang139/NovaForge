package converter

import (
	userrepo "github.com/wangliang139/NovaForge/server/pkg/repos/user"
	"github.com/wangliang139/NovaForge/server/pkg/types"
)

// UserRepo2Types 将数据库 User 转为业务 User
func UserRepo2Types(user *userrepo.User) *types.User {
	if user == nil {
		return nil
	}

	protoUser := &types.User{
		Id:   user.ID,
		Name: user.Name,
	}

	if user.Username != nil {
		protoUser.Username = *user.Username
	}
	if user.Avatar != nil {
		protoUser.Avatar = *user.Avatar
	}

	protoUser.Access = ConvertStringToAccess(user.Access)
	protoUser.Status = ConvertStringToStatus(user.Status)
	protoUser.CreatedAt = user.CreatedAt
	protoUser.UpdatedAt = user.UpdatedAt

	return protoUser
}

// ConvertStringToAccess 将字符串转换为 UserAccess 枚举
func ConvertStringToAccess(access *string) types.UserAccess {
	if access == nil {
		return types.UserAccessUser
	}
	switch *access {
	case "admin":
		return types.UserAccessAdmin
	case "user":
		return types.UserAccessUser
	default:
		return types.UserAccessUser
	}
}

// ConvertStringToStatus 将字符串转换为 UserStatus 枚举
func ConvertStringToStatus(status *string) types.UserStatus {
	if status == nil {
		return types.UserStatusActive
	}
	switch *status {
	case "active":
		return types.UserStatusActive
	case "disabled":
		return types.UserStatusDisabled
	default:
		return types.UserStatusActive
	}
}
