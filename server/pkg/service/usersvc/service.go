package usersvc

import (
	"context"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	converter "github.com/wangliang139/NovaForge/server/pkg/converter"
	"github.com/wangliang139/NovaForge/server/pkg/entity"
	"github.com/wangliang139/NovaForge/server/pkg/internal/encrypt"
	"github.com/wangliang139/NovaForge/server/pkg/repos"
	userrepo "github.com/wangliang139/NovaForge/server/pkg/repos/user"
	"github.com/wangliang139/NovaForge/server/pkg/settings"
	"github.com/wangliang139/NovaForge/server/pkg/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Service struct {
	db *repos.Entity
}

func New(db *repos.Entity) (*Service, error) {
	return &Service{
		db: db,
	}, nil
}

// Authorize 单用户模式：仅校验密码（账户为 id 最小的可用用户）。
func (s *Service) Authorize(ctx context.Context, password string) (*types.User, error) {
	if password == "" {
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}

	u, err := s.db.UserRepo.GetFirstActiveUser(ctx)
	if err != nil {
		log.Error().Err(err).Msg("single-user password auth lookup failed")
		return nil, status.Error(codes.Internal, "authentication failed")
	}
	if u == nil {
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}
	if u.PasswordHash == nil || *u.PasswordHash == "" {
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}

	stored := strings.TrimSpace(*u.PasswordHash)
	submitted := strings.TrimSpace(password)

	p1, err := encrypt.DecryptBase64(stored)
	if err != nil {
		return nil, err
	}
	p2, err := encrypt.DecryptBase64(submitted)
	if err != nil {
		return nil, err
	}
	if p1 != p2 {
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}
	return converter.UserRepo2Types(u), nil
}

// GetUserByID 根据 ID 获取用户
func (s *Service) GetUserByID(ctx context.Context, id int64) (*types.User, error) {
	if id <= 0 {
		return nil, status.Error(codes.InvalidArgument, "invalid user id")
	}

	user, err := s.db.UserRepo.GetUserByID(ctx, id)
	if err != nil {
		log.Error().Err(err).Int64("id", id).Msg("failed to get user by id")
		return nil, status.Error(codes.Internal, "failed to get user")
	}

	if user == nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	return converter.UserRepo2Types(user), nil
}

// CreateUser 创建用户
func (s *Service) CreateUser(ctx context.Context, req *types.CreateUserRequest) (*types.User, error) {
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	params := userrepo.CreateUserParams{
		Name: req.Name,
	}

	if req.Avatar != "" {
		params.Avatar = &req.Avatar
	}
	if req.Username != "" {
		params.Username = &req.Username
	}

	params.Access = lo.ToPtr(string(req.Access))
	params.Status = lo.ToPtr(string(req.Status))

	user, err := s.db.UserRepo.CreateUser(ctx, params)
	if err != nil {
		log.Error().Err(err).Msg("failed to create user")
		return nil, status.Error(codes.Internal, "failed to create user")
	}

	return converter.UserRepo2Types(user), nil
}

// UpdateUser 更新用户信息
func (s *Service) UpdateUser(ctx context.Context, req *types.UpdateUserRequest) (*types.User, error) {
	if req.Id <= 0 {
		return nil, status.Error(codes.InvalidArgument, "invalid user id")
	}

	existingUser, err := s.db.UserRepo.GetUserByID(ctx, req.Id)
	if err != nil {
		log.Error().Err(err).Int64("id", req.Id).Msg("failed to get user")
		return nil, status.Error(codes.Internal, "failed to get user")
	}
	if existingUser == nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	params := userrepo.UpdateUserParams{
		ID:       req.Id,
		Name:     &existingUser.Name,
		Avatar:   existingUser.Avatar,
		Username: existingUser.Username,
		Access:   existingUser.Access,
		Status:   existingUser.Status,
	}

	if req.Name != nil {
		params.Name = req.Name
	}
	if req.Avatar != nil {
		params.Avatar = req.Avatar
	}
	if req.Username != nil {
		params.Username = req.Username
	}
	if req.Access != nil {
		access := string(*req.Access)
		params.Access = &access
	}
	if req.Status != nil {
		statusStr := string(*req.Status)
		params.Status = &statusStr
	}

	user, err := s.db.UserRepo.UpdateUser(ctx, params)
	if err != nil {
		log.Error().Err(err).Int64("id", req.Id).Msg("failed to update user")
		return nil, status.Error(codes.Internal, "failed to update user")
	}

	return converter.UserRepo2Types(user), nil
}

// ListUsers 列出用户
func (s *Service) ListUsers(ctx context.Context, req *types.ListUsersRequest) (*types.ListUsersResponse, error) {
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize

	users, err := s.db.UserRepo.ListUsers(ctx, userrepo.ListUsersParams{
		Limit:  pageSize,
		Offset: offset,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to list users")
		return nil, status.Error(codes.Internal, "failed to list users")
	}

	protoUsers := make([]*types.User, 0, len(users))
	for _, user := range users {
		protoUsers = append(protoUsers, converter.UserRepo2Types(&user))
	}

	total := int32(len(users))

	return &types.ListUsersResponse{
		Users: protoUsers,
		Total: total,
	}, nil
}

// ChangePassword 校验当前密码后更新为新密码（与登录一致，password_hash 字段存明文比对用 secret）。
func (s *Service) ChangePassword(ctx context.Context, userID int64, currentPassword, newPassword string) error {
	currentPassword = strings.TrimSpace(currentPassword)
	newPassword = strings.TrimSpace(newPassword)
	if newPassword == "" {
		return status.Error(codes.InvalidArgument, "new password is required")
	}
	if len(newPassword) < 8 {
		return status.Error(codes.InvalidArgument, "new password must be at least 8 characters")
	}

	u, err := s.db.UserRepo.GetUserByID(ctx, userID)
	if err != nil {
		log.Error().Err(err).Int64("id", userID).Msg("change password: get user failed")
		return status.Error(codes.Internal, "failed to change password")
	}
	if u == nil {
		return status.Error(codes.NotFound, "user not found")
	}
	if u.PasswordHash == nil || *u.PasswordHash == "" {
		return status.Error(codes.FailedPrecondition, "password not set for this account")
	}
	stored := strings.TrimSpace(*u.PasswordHash)
	p1, err := encrypt.DecryptBase64(stored)
	if err != nil {
		return err
	}
	p2, err := encrypt.DecryptBase64(currentPassword)
	if err != nil {
		return err
	}
	if p1 != p2 {
		return status.Error(codes.Unauthenticated, "invalid credentials")
	}

	newHash := newPassword
	_, err = s.db.UserRepo.UpdatePasswordHash(ctx, userrepo.UpdatePasswordHashParams{
		ID:           userID,
		PasswordHash: &newHash,
	})
	if err != nil {
		log.Error().Err(err).Int64("id", userID).Msg("change password: update failed")
		return status.Error(codes.Internal, "failed to change password")
	}
	return nil
}

func (s *Service) GetPushConfig(ctx context.Context) (settings.PushConfig, error) {
	return settings.GetPushConfig(ctx)
}

func (s *Service) UpdatePushConfig(ctx context.Context, st settings.PushConfig) error {
	return entity.User.UpdatePushConfig(ctx, st)
}

func (s *Service) GetLlmProviderConfig(ctx context.Context) (settings.LlmProviderConfig, error) {
	return settings.GetLlmProviderConfig(ctx)
}

func (s *Service) UpdateLlmProviderConfig(ctx context.Context, st settings.LlmProviderConfig) error {
	return entity.User.UpdateLlmProviderConfig(ctx, st)
}

func (s *Service) GetSettings(ctx context.Context, keys []string) ([]settings.SettingEntry, error) {
	return settings.GetSettings(ctx, keys)
}

func (s *Service) SetSetting(ctx context.Context, key, value string) error {
	return settings.Set(ctx, key, value)
}

// GetNetworkProxyURL 返回用户配置的 HTTP(S) 代理 URL，未设置时为空串。
func (s *Service) GetNetworkProxyURL(ctx context.Context) (string, error) {
	return settings.GetHttpProxyURL(ctx)
}

// UpdateNetworkProxyURL 更新代理 URL 并丢弃已缓存的交易所连接器。
func (s *Service) UpdateNetworkProxyURL(ctx context.Context, raw string) error {
	if err := settings.SetHttpProxyURL(ctx, raw); err != nil {
		return err
	}
	return nil
}
