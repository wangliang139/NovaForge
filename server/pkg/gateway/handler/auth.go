package handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/wangliang139/llt-trade/server/pkg/gateway/auth"
	"github.com/wangliang139/llt-trade/server/pkg/gateway/gql"
	"github.com/wangliang139/llt-trade/server/pkg/service/usersvc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AuthHandler 认证处理器
type AuthHandler struct {
	userSvc *usersvc.Service
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(userSvc *usersvc.Service) *AuthHandler {
	return &AuthHandler{
		userSvc: userSvc,
	}
}

type loginRequest struct {
	Password string `json:"password"`
}

// Login 单用户模式：仅校验密码
// POST /api/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	ctx := c.Request.Context()

	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		gql.WrapGinError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Password == "" {
		gql.WrapGinError(c, http.StatusBadRequest, "password is required")
		return
	}

	user, err := h.userSvc.Authorize(ctx, req.Password)
	if err != nil {
		st, ok := status.FromError(err)
		if ok && st.Code() == codes.Unauthenticated {
			gql.WrapGinError(c, http.StatusUnauthorized, "invalid password")
			return
		}
		log.Error().Err(err).Msg("login failed")
		gql.WrapGinError(c, http.StatusInternalServerError, "login failed")
		return
	}

	jwtToken, err := auth.GenerateToken(user.Id, user.Name, string(user.Access))
	if err != nil {
		log.Error().Err(err).Msg("failed to generate token")
		gql.WrapGinError(c, http.StatusInternalServerError, "login failed")
		return
	}

	gql.WrapGinResponse(c, map[string]string{
		"token": jwtToken,
	})
}

// GetCurrentUser 获取当前登录用户信息
// GET /api/auth/me
func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	ctx := c.Request.Context()

	user, ok := auth.GetUserFromContext(ctx)
	if !ok {
		gql.WrapGinError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	name := user.Name
	access := user.Access
	avatar := ""

	userBo, err := h.userSvc.GetUserByID(ctx, user.ID)
	if err == nil && userBo != nil {
		if userBo.Avatar != "" {
			avatar = userBo.Avatar
		}
		if userBo.Name != "" {
			name = userBo.Name
		}
		access = string(userBo.Access)
	} else if err != nil {
		log.Warn().Err(err).Int64("user_id", user.ID).Msg("failed to get user from user service")
	}

	gql.WrapGinResponse(c, map[string]string{
		"userid": fmt.Sprintf("%d", user.ID),
		"email":  "",
		"name":   name,
		"access": access,
		"avatar": avatar,
	})
}

// Logout 退出登录
// POST /api/auth/logout
func (h *AuthHandler) Logout(c *gin.Context) {
	gql.WrapGinResponse(c, map[string]any{
		"success": true,
		"message": "logout successful",
	})
}
