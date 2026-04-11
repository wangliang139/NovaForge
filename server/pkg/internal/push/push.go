package push

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/microcosm-cc/bluemonday"
	"github.com/mymmrac/telego"
	"github.com/wangliang139/NovaForge/server/pkg/internal/tgbot"
	"github.com/wangliang139/NovaForge/server/pkg/settings"
	"github.com/wangliang139/mow/logger"
)

var feishuHTTPClient = &http.Client{Timeout: 10 * time.Second}

type Scene string

const (
	SceneDocument Scene = "document"
	SceneAlarm    Scene = "alarm"
	SceneTrade    Scene = "trade"
)

type NotifyRequest struct {
	SceneKey string
	Message  string
}

type NotifyByTemplateRequest struct {
	SceneKey string
	Vars     map[string]any
}

func Notify(ctx context.Context, req NotifyRequest) error {
	if strings.TrimSpace(req.Message) == "" {
		return nil
	}
	cfg, err := settings.GetPushConfig(ctx)
	if err != nil {
		return err
	}
	if !sceneEnabled(cfg, req.SceneKey) {
		return nil
	}
	return sendByProvider(ctx, cfg, req.Message)
}

func NotifyByTemplate(ctx context.Context, req NotifyByTemplateRequest) error {
	cfg, err := settings.GetPushConfig(ctx)
	if err != nil {
		return err
	}
	if !sceneEnabled(cfg, req.SceneKey) {
		return nil
	}
	provider := strings.TrimSpace(cfg.Provider)
	if provider == "" {
		provider = settings.PushProviderTelegram
	}
	tpl, ok, err := settings.GetPushTemplate(ctx, req.SceneKey, provider)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("push template not found")
	}
	message := renderTemplate(tpl, req.Vars)
	if strings.TrimSpace(message) == "" {
		return errors.New("push template rendered empty")
	}
	return sendByProvider(ctx, cfg, message)
}

func sceneEnabled(cfg settings.PushConfig, sceneKey string) bool {
	var scene string
	sceneKeyParts := strings.Split(sceneKey, ".")
	if len(sceneKeyParts) > 1 {
		scene = sceneKeyParts[0]
	} else {
		scene = sceneKey
	}
	switch scene {
	case string(SceneDocument):
		return cfg.PushDocumentEnabled
	case string(SceneAlarm):
		return cfg.PushAlarmEnabled
	case string(SceneTrade):
		return cfg.PushTradeEnabled
	default:
		return true
	}
}

func sendByProvider(ctx context.Context, cfg settings.PushConfig, message string) error {
	provider := strings.TrimSpace(cfg.Provider)
	if provider == "" {
		provider = settings.PushProviderTelegram
	}
	switch provider {
	case settings.PushProviderFeishu:
		return sendFeishuText(ctx, cfg, htmlToText(message))
	default:
		return sendTelegramHTML(ctx, cfg, message)
	}
}

func sendTelegramHTML(ctx context.Context, cfg settings.PushConfig, message string) error {
	if !tgbot.HasGlobalClient() {
		return nil
	}
	chatID := cfg.TelegramChannelID
	if chatID == 0 {
		return nil
	}
	_, err := tgbot.SendMessage(ctx, &telego.SendMessageParams{
		ChatID: telego.ChatID{ID: int64(chatID)},
		Text:   message,
		LinkPreviewOptions: &telego.LinkPreviewOptions{
			IsDisabled: true,
		},
		ParseMode: string(tgbot.ParseModeHTML),
	})
	return err
}

func sendFeishuText(ctx context.Context, cfg settings.PushConfig, text string) error {
	webhookURL := strings.TrimSpace(cfg.FeishuWebhookURL)
	if webhookURL == "" {
		return nil
	}
	payload := map[string]any{
		"msg_type": "text",
		"content": map[string]string{
			"text": buildFeishuText(cfg, text),
		},
	}

	secret := strings.TrimSpace(cfg.FeishuSecret)
	if secret != "" {
		ts := fmt.Sprintf("%d", time.Now().Unix())
		sign, err := feishuSign(ts, secret)
		if err != nil {
			return err
		}
		payload["timestamp"] = ts
		payload["sign"] = sign
	}

	body, err := sonic.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := feishuHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("feishu webhook status: %d", resp.StatusCode)
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	logger.Ctx(ctx).Info().Str("response", string(responseBody)).Msg("feishu webhook response")

	var response struct {
		Code    int    `json:"code"`
		Data    any    `json:"data"`
		Message string `json:"message"`
	}
	err = sonic.Unmarshal(responseBody, &response)
	if err != nil {
		return err
	}
	if response.Code != 0 {
		return fmt.Errorf("feishu webhook response: %s", response.Message)
	}

	return nil
}

func htmlToText(s string) string {
	stripped := bluemonday.StrictPolicy().Sanitize(s)
	return strings.TrimSpace(html.UnescapeString(stripped))
}

func buildFeishuText(cfg settings.PushConfig, text string) string {
	parts := make([]string, 0, 2)
	keyword := strings.TrimSpace(cfg.FeishuKeyword)
	if keyword != "" {
		parts = append(parts, keyword)
	}
	if strings.TrimSpace(text) != "" {
		parts = append(parts, strings.TrimSpace(text))
	}
	if len(parts) == 0 {
		return "NovaForge 推送"
	}
	return strings.Join(parts, "\n")
}

func feishuSign(timestamp, secret string) (string, error) {
	// 与飞书文档一致：以 timestamp+"\n"+secret 为 HMAC 密钥，对空内容签名（非以 secret 为密钥）。
	stringToSign := timestamp + "\n" + secret
	h := hmac.New(sha256.New, []byte(stringToSign))
	if _, err := h.Write(nil); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(h.Sum(nil)), nil
}
