package chat

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/gin-gonic/gin"
	"github.com/wangliang139/llt-trade/server/pkg/chat/domain"
	"github.com/wangliang139/llt-trade/server/pkg/gateway/auth"
	"github.com/wangliang139/llt-trade/server/pkg/gateway/gql"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Register(r gin.IRoutes) {
	r.GET("/models", h.ListModels)
	r.GET("/sessions", h.ListSessions)
	r.GET("/sessions/:id", h.GetSession)
	r.PATCH("/sessions/:id/title", h.UpdateSessionTitle)
	r.POST("/sessions/:id/title/generate", h.GenerateSessionTitle)
	r.DELETE("/sessions/:id", h.DeleteSession)
	r.POST("/stream", h.ChatStream)
}

type updateSessionTitleBody struct {
	Title string `json:"title"`
}

type chatRequestBody struct {
	SessionID  string `json:"sessionId"`
	DialogID   string `json:"dialogId"`
	Regenerate bool   `json:"regenerate"`
	Content    string `json:"content"`
	Model      string `json:"model"`
}

func (h *Handler) ListModels(c *gin.Context) {
	if _, ok := auth.GetUserFromContext(c.Request.Context()); !ok {
		gql.WrapGinError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	items, err := h.svc.ListModels(c.Request.Context())
	if err != nil {
		gql.WrapGinError(c, http.StatusBadGateway, err.Error())
		return
	}
	gql.WrapGinResponse(c, items)
}

func (h *Handler) ListSessions(c *gin.Context) {
	user, ok := auth.GetUserFromContext(c.Request.Context())
	if !ok {
		gql.WrapGinError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	limit := int32(20)
	offset := int32(0)
	if raw := c.Query("limit"); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 32); err == nil {
			limit = int32(v)
		}
	}
	if raw := c.Query("offset"); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 32); err == nil {
			offset = int32(v)
		}
	}

	resp, err := h.svc.ListSessions(c.Request.Context(), user.ID, limit, offset)
	if err != nil {
		gql.WrapGinError(c, http.StatusInternalServerError, err.Error())
		return
	}
	gql.WrapGinResponse(c, resp)
}

func (h *Handler) GetSession(c *gin.Context) {
	user, ok := auth.GetUserFromContext(c.Request.Context())
	if !ok {
		gql.WrapGinError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	sessionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || sessionID <= 0 {
		gql.WrapGinError(c, http.StatusBadRequest, "invalid session id")
		return
	}
	resp, err := h.svc.GetSession(c.Request.Context(), user.ID, sessionID)
	if err != nil {
		code := http.StatusInternalServerError
		if err.Error() == "session not found" {
			code = http.StatusNotFound
		}
		if err.Error() == "forbidden" {
			code = http.StatusForbidden
		}
		gql.WrapGinError(c, code, err.Error())
		return
	}
	gql.WrapGinResponse(c, resp)
}

func (h *Handler) DeleteSession(c *gin.Context) {
	user, ok := auth.GetUserFromContext(c.Request.Context())
	if !ok {
		gql.WrapGinError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	sessionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || sessionID <= 0 {
		gql.WrapGinError(c, http.StatusBadRequest, "invalid session id")
		return
	}
	if err := h.svc.DeleteSession(c.Request.Context(), user.ID, sessionID); err != nil {
		code := http.StatusInternalServerError
		if err.Error() == "forbidden" {
			code = http.StatusForbidden
		}
		gql.WrapGinError(c, code, err.Error())
		return
	}
	gql.WrapGinResponse(c, true)
}

func (h *Handler) GenerateSessionTitle(c *gin.Context) {
	user, ok := auth.GetUserFromContext(c.Request.Context())
	if !ok {
		gql.WrapGinError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	sessionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || sessionID <= 0 {
		gql.WrapGinError(c, http.StatusBadRequest, "invalid session id")
		return
	}
	title, err := h.svc.GenerateSessionTitle(c.Request.Context(), user.ID, sessionID)
	if err != nil {
		code := http.StatusInternalServerError
		if err.Error() == "session not found" {
			code = http.StatusNotFound
		}
		if err.Error() == "forbidden" {
			code = http.StatusForbidden
		}
		gql.WrapGinError(c, code, err.Error())
		return
	}
	gql.WrapGinResponse(c, map[string]string{
		"title": title,
	})
}

func (h *Handler) UpdateSessionTitle(c *gin.Context) {
	user, ok := auth.GetUserFromContext(c.Request.Context())
	if !ok {
		gql.WrapGinError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	sessionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || sessionID <= 0 {
		gql.WrapGinError(c, http.StatusBadRequest, "invalid session id")
		return
	}
	var body updateSessionTitleBody
	if err := c.ShouldBindJSON(&body); err != nil {
		gql.WrapGinError(c, http.StatusBadRequest, "invalid request")
		return
	}
	title, err := h.svc.UpdateSessionTitle(c.Request.Context(), user.ID, sessionID, body.Title)
	if err != nil {
		code := http.StatusInternalServerError
		if err.Error() == "session not found" {
			code = http.StatusNotFound
		}
		if err.Error() == "forbidden" {
			code = http.StatusForbidden
		}
		gql.WrapGinError(c, code, err.Error())
		return
	}
	gql.WrapGinResponse(c, map[string]string{
		"title": title,
	})
}

func (h *Handler) ChatStream(c *gin.Context) {
	// 用户鉴权
	user, ok := auth.GetUserFromContext(c.Request.Context())
	if !ok {
		gql.WrapGinError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	// 解析请求体
	var body chatRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		gql.WrapGinError(c, http.StatusBadRequest, "invalid request")
		return
	}

	var (
		sid int64
		did int64
		err error
	)
	if body.SessionID != "" {
		sid, err = strconv.ParseInt(body.SessionID, 10, 64)
		if err != nil {
			gql.WrapGinError(c, http.StatusBadRequest, "invalid sessionId")
			return
		}
	}
	if body.DialogID != "" {
		did, err = strconv.ParseInt(body.DialogID, 10, 64)
		if err != nil {
			gql.WrapGinError(c, http.StatusBadRequest, "invalid dialogId")
			return
		}
	}

	req := domain.ChatRequest{
		UserID:     user.ID,
		SessionID:  sid,
		DialogID:   did,
		Regenerate: body.Regenerate,
		Content:    body.Content,
		Model:      body.Model,
	}

	events, err := h.svc.ChatStream(c.Request.Context(), req)
	if err != nil {
		code := http.StatusBadRequest
		switch err.Error() {
		case "session not found":
			code = http.StatusNotFound
		case "forbidden":
			code = http.StatusForbidden
		case "only latest answer can regenerate":
			code = http.StatusConflict
		case "content is required":
			code = http.StatusBadRequest
		}
		if strings.Contains(err.Error(), "answer dialog not found") || strings.Contains(err.Error(), "answer is already streaming") {
			code = http.StatusBadRequest
		}
		gql.WrapGinError(c, code, err.Error())
		return
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		gql.WrapGinError(c, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	flusher.Flush()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			if err := writeSseEvent(c.Writer, flusher, event); err != nil {
				return
			}
		}
	}
}

func writeSseEvent(w http.ResponseWriter, flusher http.Flusher, event domain.DeltaEvent) error {
	payload, err := sonic.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "id: %s\n", event.ID); err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, "event: message\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}
