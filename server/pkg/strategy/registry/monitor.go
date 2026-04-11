package registry

import (
	"context"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/wangliang139/NovaForge/server/pkg/internal/push"
	"github.com/wangliang139/NovaForge/server/pkg/repos/bot"
	"github.com/wangliang139/NovaForge/server/pkg/strategy"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	"github.com/wangliang139/mow/logger"
)

type ActionLevel int

const (
	ActionLevelAlarm    ActionLevel = 1
	ActionLevelShutdown ActionLevel = 2
	ActionLevelNone     ActionLevel = 0
)

func (l ActionLevel) String() string {
	switch l {
	case ActionLevelAlarm:
		return "ALARM"
	case ActionLevelShutdown:
		return "SHUTDOWN"
	default:
		return "NONE"
	}
}

func buildAlarmArgs(bot *bot.Bot, level ActionLevel, reasons []string) map[string]any {
	escapedReasons := make([]string, 0, len(reasons))
	for _, r := range reasons {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		escapedReasons = append(escapedReasons, html.EscapeString(r))
	}

	var reasonLines string
	if len(escapedReasons) == 0 {
		reasonLines = "无详细原因"
	} else {
		reasonLines = "- " + strings.Join(escapedReasons, "\n- ")
	}
	return map[string]any{
		"botId":   bot.ID,
		"botName": bot.Name,
		"level":   level.String(),
		"reasons": reasonLines,
		"time":    time.Now().Format(time.RFC3339),
	}
}

// checkExecutorsHealth 遍历当前所有执行器并检测运行状态
func (r *ExecutorRegistry) checkExecutorsHealth(ctx context.Context) {
	r.mu.RLock()
	executors := make([]strategy.Executor, 0, len(r.executors))
	botIDs := make([]int32, 0, len(r.executors))
	for id, exec := range r.executors {
		botIDs = append(botIDs, id)
		executors = append(executors, exec)
	}
	r.mu.RUnlock()

	if len(executors) == 0 {
		return
	}

	type action struct {
		level  ActionLevel
		reason string
	}

	for i, exec := range executors {
		if exec == nil {
			continue
		}
		botID := botIDs[i]

		state := exec.GetState()
		if state == nil {
			logger.Ctx(ctx).Error().Int32("bot_id", botID).Msg("failed to get executor state")
			continue
		}

		now := time.Now()

		actions := make([]action, 0)

		status := state.Status
		runErr := state.RunErr
		jsStatus := state.JsRunnerStatus
		lastSignalTs := state.LastSignalTs

		var reasons []string
		if status != stypes.ExecutorStatusRunning {
			actions = append(actions, action{
				level:  ActionLevelShutdown,
				reason: fmt.Sprintf("status=%s", status),
			})
		}
		if runErr != nil {
			actions = append(actions, action{
				level:  ActionLevelShutdown,
				reason: fmt.Sprintf("runErr=%s", runErr.Error()),
			})
		}
		if jsStatus != "" && jsStatus != "running" {
			actions = append(actions, action{
				level:  ActionLevelShutdown,
				reason: fmt.Sprintf("jsRunnerStatus=%s", jsStatus),
			})
		}
		if r.cfg.SignalAlarmTimeout > 0 {
			last := time.UnixMilli(lastSignalTs)
			if now.Sub(last) > r.cfg.SignalShutdownTimeout {
				actions = append(actions, action{
					level:  ActionLevelShutdown,
					reason: fmt.Sprintf("no signal for %s", now.Sub(last)),
				})
			} else if now.Sub(last) > r.cfg.SignalAlarmTimeout {
				actions = append(actions, action{
					level:  ActionLevelAlarm,
					reason: fmt.Sprintf("no signal for %s", now.Sub(last)),
				})
			}
		}

		if len(actions) == 0 {
			continue
		}

		maxLevel := ActionLevelNone
		for _, action := range actions {
			if action.level > maxLevel {
				maxLevel = action.level
			}
			reasons = append(reasons, action.reason)
		}

		logger.Ctx(ctx).Warn().
			Int32("bot_id", botID).
			Str("reason", strings.Join(reasons, "; ")).
			Msg("executor health check failed, stopping executor")

		if !r.cfg.EnableAlarm {
			go func() {
				r.sendAlarmMessage(ctx, botID, maxLevel, reasons)
			}()
		}

		if maxLevel == ActionLevelShutdown {
			r.handleAbnormalExecutor(ctx, botID, strings.Join(reasons, "; "))
		}
	}
}

func (r *ExecutorRegistry) sendAlarmMessage(ctx context.Context, botID int32, level ActionLevel, reasons []string) {
	bot, err := r.db.BotRepo.GetBot(ctx, botID)
	if err != nil {
		logger.Ctx(ctx).Warn().Err(err).Int32("bot_id", botID).Msg("failed to get bot")
		return
	}
	if bot == nil {
		logger.Ctx(ctx).Warn().Int32("bot_id", botID).Msg("bot is nil")
		return
	}

	err = push.NotifyByTemplate(ctx, push.NotifyByTemplateRequest{
		SceneKey: "alarm.bot.monitor",
		Vars: buildAlarmArgs(bot, level, reasons),
	})
	if err != nil {
		logger.Ctx(ctx).Warn().Err(err).Msg("failed to send alarm push")
	}
}

// handleAbnormalExecutor 停止异常执行器并将 Bot 状态标记为 error
func (r *ExecutorRegistry) handleAbnormalExecutor(ctx context.Context, botID int32, reason string) {
	// 确认当前仍有执行器在运行，避免重复处理
	r.mu.RLock()
	_, exists := r.executors[botID]
	r.mu.RUnlock()
	if !exists {
		return
	}

	if err := r.Stop(ctx, botID); err != nil {
		logger.Ctx(ctx).Warn().
			Err(err).
			Int32("bot_id", botID).
			Msg("failed to stop abnormal executor")
	}

	if r.db == nil || r.db.BotRepo == nil {
		logger.Ctx(ctx).Warn().
			Int32("bot_id", botID).
			Str("reason", reason).
			Msg("bot repo is nil, cannot update bot status to error")
		return
	}

	const maxReasonLen = 1024
	if len(reason) > maxReasonLen {
		reason = reason[:maxReasonLen]
	}

	_, err := r.db.BotRepo.UpdateBotStatus(ctx, bot.UpdateBotStatusParams{
		ID:           botID,
		Status:       bot.BotStatus(stypes.BotStatusError),
		ErrorMessage: reason,
	})
	if err != nil {
		logger.Ctx(ctx).Warn().
			Err(err).
			Int32("bot_id", botID).
			Str("reason", reason).
			Msg("failed to update bot status to error for abnormal executor")
		return
	}

	logger.Ctx(ctx).Info().
		Int32("bot_id", botID).
		Str("reason", reason).
		Msg("abnormal executor stopped and bot status updated to error")
}
