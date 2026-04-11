package tgbotsvc

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/bytedance/sonic"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"github.com/wangliang139/NovaForge/server/pkg/entity"
	"github.com/wangliang139/NovaForge/server/pkg/repos"
	"github.com/wangliang139/NovaForge/server/pkg/settings"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/mow/logger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const TracerName = "github.com/wangliang139/NovaForge/server/pkg/service/tgbot"

// configPollInterval KV 中资讯采集配置轮询间隔；变更后至多延迟该时间重新订阅。
const configPollInterval = 30 * time.Second

var tracer = otel.Tracer(TracerName)

type Service struct {
	cache redis.UniversalClient
	db    *repos.Entity

	tgClient *telegram.Client

	ctx      context.Context
	cancelFn context.CancelFunc

	clientMu sync.RWMutex
	idleWg   sync.WaitGroup
}

func New(cache redis.UniversalClient, db *repos.Entity) (*Service, error) {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		cache:    cache,
		db:       db,
		ctx:      ctx,
		cancelFn: cancel,
	}, nil
}

func (s *Service) Close() {
	s.cancelFn()
	s.clientMu.Lock()
	if s.tgClient != nil {
		s.tgClient.Stop()
		s.tgClient = nil
	}
	s.clientMu.Unlock()
	s.idleWg.Wait()
}

func collectorFingerprint(enabled bool, c settings.TelegramAppConfig) string {
	switchSig := "0"
	if enabled {
		switchSig = "1"
	}
	return switchSig + "\x1e" + strings.TrimSpace(c.AppID) + "\x1e" + strings.TrimSpace(c.AppHash) + "\x1e" + strings.TrimSpace(c.Session)
}

func collectorConfigReady(c settings.TelegramAppConfig) bool {
	return strings.TrimSpace(c.AppID) != "" &&
		strings.TrimSpace(c.AppHash) != "" &&
		strings.TrimSpace(c.Session) != ""
}

// initClientFromKV 使用 settings.TelegramNewsCollector 解析出的 TelegramAppConfig 创建并连接 MTProto 客户端。
func (s *Service) initClientFromKV(cfg settings.TelegramAppConfig) error {
	appIDStr := strings.TrimSpace(cfg.AppID)
	appHash := strings.TrimSpace(cfg.AppHash)
	session := strings.TrimSpace(cfg.Session)
	if !collectorConfigReady(cfg) {
		return errors.New("telegram news collector config incomplete (app_id, app_hash, session required)")
	}
	appIDInt, err := strconv.Atoi(appIDStr)
	if err != nil {
		return fmt.Errorf("invalid app id: %w", err)
	}

	var proxy telegram.Proxy
	httpProxyURL, err := settings.GetHttpProxyURL(context.Background())
	if err != nil {
		return fmt.Errorf("get http proxy url: %w", err)
	}
	if len(httpProxyURL) > 0 {
		proxy, err = telegram.ProxyFromURL(httpProxyURL)
		if err != nil {
			return fmt.Errorf("failed to parse proxy url: %w", err)
		}
	}

	var cerr error
	s.tgClient, cerr = telegram.NewClient(telegram.ClientConfig{
		AppID:         int32(appIDInt),
		AppHash:       appHash,
		StringSession: session,
		Proxy:         proxy,
		MemorySession: true,
		ErrorHandler: func(err error) bool {
			log.Error().Err(err).Msg("telegram error")
			return false
		},
		FloodHandler: func(err error) bool {
			log.Error().Err(err).Msg("telegram flood error")
			return false
		},
		LogLevel:   1,
		Timeout:    10,
		ReqTimeout: 10,
		Cache: telegram.NewCache("cache.db", &telegram.CacheConfig{
			MaxSize:  1000, // TODO
			LogLevel: telegram.LogInfo,
			LogColor: false,
			Memory:   true,
			Disabled: false,
		}),
	})
	if cerr != nil {
		log.Error().Err(cerr).Msg("failed to create telegram client")
		return cerr
	}

	if cerr = s.tgClient.Connect(); cerr != nil {
		log.Error().Err(cerr).Msg("failed to connect to telegram")
		s.tgClient = nil
		return cerr
	}
	return nil
}

func (s *Service) stopClientLocked() {
	if s.tgClient != nil {
		s.tgClient.Stop()
		s.tgClient = nil
	}
	s.idleWg.Wait()
}

func (s *Service) registerMessageHandlers() {
	s.tgClient.On(telegram.OnMessage, func(message *telegram.NewMessage) error {
		go s.handleMessage("message", message)
		return nil
	}, telegram.FilterChannel)
	s.tgClient.On(telegram.OnCommand, func(message *telegram.NewMessage) error {
		go s.handleMessage("command", message)
		return nil
	}, telegram.FilterChannel)
	s.tgClient.On(telegram.OnEdit, func(message *telegram.NewMessage) error {
		go s.handleMessage("edit", message)
		return nil
	}, telegram.FilterChannel)
	s.tgClient.On(telegram.OnAlbum, func(message *telegram.Album) error {
		for _, msg := range message.Messages {
			go s.handleMessage("album", msg)
		}
		return nil
	}, telegram.FilterChannel)
}

// applyConfig 在配置指纹变化时调用：先停止旧客户端与订阅，再按新 TelegramAppConfig 连接并重新注册监听。
func (s *Service) applyConfig(enabled bool, cfg settings.TelegramAppConfig) {
	s.clientMu.Lock()
	s.stopClientLocked()

	if !enabled {
		s.clientMu.Unlock()
		log.Info().Msg("telegram news collector: 已关闭采集开关，已停止监听")
		return
	}

	if !collectorConfigReady(cfg) {
		s.clientMu.Unlock()
		log.Info().Msg("telegram news collector: 配置为空或不完整，已停止监听")
		return
	}

	if err := s.initClientFromKV(cfg); err != nil {
		s.clientMu.Unlock()
		log.Err(err).Msg("telegram news collector: 初始化客户端失败")
		return
	}

	s.registerMessageHandlers()
	client := s.tgClient
	s.idleWg.Add(1)
	s.clientMu.Unlock()

	go func() {
		defer s.idleWg.Done()
		if client != nil {
			client.Idle()
		}
		log.Warn().Msg("telegram news collector: client idle 已退出")
	}()

	log.Info().Msg("Telegram 资讯采集监听已启动（配置已应用）")
}

func (s *Service) runConfigWatcher() {
	ticker := time.NewTicker(configPollInterval)
	defer ticker.Stop()

	lastSig := ""
	for {
		if s.ctx.Err() != nil {
			return
		}

		cfg, err := settings.GetNewsCollectConfig(s.ctx)
		if err != nil {
			log.Err(err).Msg("TelegramNewsCollector: 读取 KV 失败")
		} else {
			enabled, eerr := settings.GetNewsCollectEnabled(s.ctx)
			if eerr != nil {
				log.Err(eerr).Msg("TelegramNewsCollector: 读取采集开关失败")
				enabled = true
			}
			sig := collectorFingerprint(enabled, cfg)
			if sig != lastSig {
				log.Info().Msg("TelegramNewsCollector: 配置变更，重新订阅")
				lastSig = sig
				s.applyConfig(enabled, cfg)
			}
		}

		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) Start() {
	go s.runConfigWatcher()
}

func (s *Service) handleMessage(msgType string, message *telegram.NewMessage) {
	s.clientMu.RLock()
	client := s.tgClient
	s.clientMu.RUnlock()
	if client == nil {
		return
	}

	chatType := message.ChatType()
	if chatType != telegram.EntityChannel && message.Channel == nil {
		log.Info().Int64("msg_id", int64(message.ID)).Msg("not a channel message")
		return
	}

	var err error
	var dbID *string
	ctx := context.Background()
	ctx, span := tracer.Start(ctx, "tgbot.message.consume")
	defer func() {
		span.SetAttributes(attribute.Int64("msg_id", int64(message.ID)))
		span.SetAttributes(attribute.Int64("chat_id", int64(message.ChatID())))
		span.SetAttributes(attribute.String("chat_type", message.ChatType()))
		span.SetAttributes(attribute.String("msg_type", msgType))
		span.SetAttributes(attribute.String("message", message.RawText(false)))
		if dbID != nil {
			span.SetAttributes(attribute.String("doc_id", *dbID))
		}
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "success")
		}
		span.End()
	}()

	// 用 cache 做幂等性处理
	cacheKey := fmt.Sprintf("tgbot:message:%d", message.ID)
	exists, err := s.cache.SetNX(ctx, cacheKey, 1, time.Hour*1).Result()
	if err != nil {
		logger.Ctx(ctx).Err(err).Msg("failed to set cache")
		return
	}
	if !exists {
		logger.Ctx(ctx).Info().Int64("msg_id", int64(message.ID)).Msg("message already processed")
		return
	}

	channel, err := client.GetChannel(message.ChatID())
	if err != nil {
		logger.Ctx(ctx).Err(err).Msg("failed to get channel")
		return
	}

	go func() {
		cnl := struct {
			ID        int64  `json:"id"`
			Title     string `json:"title"`
			Username  string `json:"username"`
			Broadcast bool   `json:"broadcast"`
			Verified  bool   `json:"verified"`
		}{
			ID:        channel.ID,
			Title:     channel.Title,
			Username:  channel.Username,
			Broadcast: channel.Broadcast,
			Verified:  channel.Verified,
		}
		json, err2 := sonic.MarshalString(cnl)
		if err2 != nil {
			logger.Ctx(ctx).Err(err2).Msg("failed to marshal channel")
			return
		}
		_, err2 = s.cache.HSet(ctx, "tgbot:channels", fmt.Sprintf("%d", cnl.ID), json).Result()
		if err2 != nil {
			logger.Ctx(ctx).Err(err2).Msg("failed to set channel")
			return
		}
	}()

	dbID, err = entity.Document.CosumeTgSubscribedMessage(ctx, message, channel)
	if err != nil {
		logger.Ctx(ctx).Err(err).Msg("failed to consume message")
		return
	}
}

func (s *Service) SendCode(ctx context.Context, req *ctypes.SendCodeRequest) (*ctypes.SendCodeResponse, error) {
	phoneNumber := req.PhoneNumber
	if phoneNumber == "" {
		return &ctypes.SendCodeResponse{
			Success: false,
			Message: "phone number is required",
		}, nil
	}

	appID := req.AppId
	appHash := req.AppHash
	if appID == "" || appHash == "" {
		return &ctypes.SendCodeResponse{
			Success: false,
			Message: "app id and app hash are required",
		}, nil
	}
	appIDInt, err := strconv.Atoi(appID)
	if err != nil {
		return &ctypes.SendCodeResponse{
			Success: false,
			Message: "invalid app id",
		}, nil
	}

	log.Info().Int("appId", appIDInt).Str("appHash", appHash).Str("phoneNumber", phoneNumber).Msg("sending code")
	client, err := telegram.NewClient(telegram.ClientConfig{
		AppID:         int32(appIDInt),
		AppHash:       appHash,
		MemorySession: true,
		LogLevel:      1,
		ErrorHandler: func(err error) bool {
			log.Error().Err(err).Msg("telegram error")
			return false
		},
		FloodHandler: func(err error) bool {
			log.Error().Err(err).Msg("telegram flood error")
			return false
		},
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to create telegram client")
		return nil, err
	}

	err = client.Connect()
	if err != nil {
		log.Error().Err(err).Msg("failed to connect to telegram")
		return nil, err
	}

	codeHash, err := client.SendCode(phoneNumber)
	if err != nil {
		log.Error().Err(err).Msg("failed to send code")
		return &ctypes.SendCodeResponse{
			Success: false,
			Message: fmt.Sprintf("failed to send code: %v", err),
		}, nil
	}

	log.Info().Str("phone", phoneNumber).Msg("code sent successfully")

	session := client.ExportSession()
	log.Info().Str("session", session).Msg("session")

	return &ctypes.SendCodeResponse{
		Success:  true,
		CodeHash: codeHash,
		Session:  session,
		Message:  "code sent successfully",
	}, nil
}

func (s *Service) GetSession(ctx context.Context, req *ctypes.GetSessionRequest) (*ctypes.GetSessionResponse, error) {
	phoneNumber := req.PhoneNumber
	codeHash := req.CodeHash
	code := req.Code
	appID := req.AppId
	appHash := req.AppHash
	session := req.Session

	if phoneNumber == "" || codeHash == "" || code == "" || appID == "" || appHash == "" {
		return &ctypes.GetSessionResponse{
			Success: false,
			Message: "appId, appHash, phone number, code hash and code are all required",
		}, nil
	}

	appIDInt, err := strconv.Atoi(appID)
	if err != nil {
		return &ctypes.GetSessionResponse{
			Success: false,
			Message: fmt.Sprintf("invalid appId: %v", err),
		}, nil
	}

	log.Info().Int("appId", appIDInt).Str("appHash", appHash).Str("phoneNumber", phoneNumber).Str("codeHash", codeHash).Str("code", code).Msg("verifying code")
	client, err := telegram.NewClient(telegram.ClientConfig{
		AppID:         int32(appIDInt),
		AppHash:       appHash,
		MemorySession: true,
		StringSession: session,
		LogLevel:      1,
		ErrorHandler: func(err error) bool {
			log.Error().Err(err).Msg("telegram error")
			return false
		},
		FloodHandler: func(err error) bool {
			log.Error().Err(err).Msg("telegram flood error")
			return false
		},
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to create telegram client")
		return nil, err
	}

	ok, err := client.Login(phoneNumber, &telegram.LoginOptions{
		CodeHash: codeHash,
		Code:     code,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to login")
		return &ctypes.GetSessionResponse{
			Success: false,
			Message: fmt.Sprintf("login failed: %v", err),
		}, nil
	}
	if !ok {
		log.Error().Msg("login failed, code may be incorrect")
		return &ctypes.GetSessionResponse{
			Success: false,
			Message: "verification code is incorrect",
		}, nil
	}

	session2 := client.ExportSession()
	log.Info().Str("phone", phoneNumber).Msg("session created successfully")

	return &ctypes.GetSessionResponse{
		Success: true,
		Session: session2,
		Message: "session created successfully",
	}, nil
}
