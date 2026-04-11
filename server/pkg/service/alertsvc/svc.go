package alertsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
	"github.com/stumble/wpgx"
	"github.com/wangliang139/NovaForge/server/pkg/internal/push"
	"github.com/wangliang139/NovaForge/server/pkg/repos"
	repoalert "github.com/wangliang139/NovaForge/server/pkg/repos/alert"
	repoevent "github.com/wangliang139/NovaForge/server/pkg/repos/alert_trigger_event"
	"github.com/wangliang139/NovaForge/server/pkg/service/streamsvc"
	"github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/utils"
	mowerror "github.com/wangliang139/mow/errors"
)

const (
	maxAlertsPerSymbol = 20
	maxAlertsGlobal    = 200
	defaultCooldownSec = 60
)

type AddAlertInput struct {
	Exchange        types.Exchange
	Symbol          string
	Type            string
	Frequency       string
	Price           *string
	Window          *string
	Percent         *string
	Remark          *string
	CooldownSeconds *int
}

type AlertItem struct {
	ID              string
	Exchange        types.Exchange
	Symbol          string
	Type            string
	Frequency       string
	Price           *decimal.Decimal
	Window          *string
	Percent         *decimal.Decimal
	Remark          *string
	CooldownSeconds int
	Status          string
	LastTriggeredAt *time.Time
	TriggerCount    int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type watcher struct {
	exchange types.Exchange
	symbol   string
	cancel   context.CancelFunc
	done     chan struct{}
}

type Service struct {
	db          *repos.Entity
	streamSvc   *streamsvc.Service
	priceLookup marketPriceLookup

	mu       sync.RWMutex
	alerts   map[string]*runtimeAlert
	watchers map[string]*watcher

	rootCtx context.Context
	cancel  context.CancelFunc
}

func New(db *repos.Entity, streamSvc *streamsvc.Service, priceLookup marketPriceLookup) (*Service, error) {
	return &Service{
		db:          db,
		streamSvc:   streamSvc,
		priceLookup: priceLookup,
		alerts:      make(map[string]*runtimeAlert),
		watchers:    make(map[string]*watcher),
	}, nil
}

func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return nil
	}
	s.rootCtx, s.cancel = context.WithCancel(ctx)
	s.mu.Unlock()

	rows, err := s.db.AlertRepo.ListAllActiveAlerts(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		a := toAlertItem(row)
		s.addRuntimeAlert(a)
	}
	return nil
}

func (s *Service) Stop() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	watchers := make([]*watcher, 0, len(s.watchers))
	for _, w := range s.watchers {
		watchers = append(watchers, w)
	}
	s.mu.Unlock()
	for _, w := range watchers {
		<-w.done
	}
}

func (s *Service) ListAlerts(ctx context.Context, exchange types.Exchange, symbol string) ([]AlertItem, error) {
	if err := validateExchangeSymbol(exchange, symbol); err != nil {
		return nil, err
	}
	rows, err := s.db.AlertRepo.ListAlertsByExchangeSymbol(ctx, repoalert.ListAlertsByExchangeSymbolParams{
		Exchange: exchange.String(),
		Symbol:   normalizeSymbol(symbol),
	})
	if err != nil {
		return nil, err
	}
	out := make([]AlertItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAlertItem(row))
	}
	return out, nil
}

func (s *Service) AddAlert(ctx context.Context, input AddAlertInput) (*AlertItem, error) {
	if err := validateAddInput(input); err != nil {
		return nil, err
	}
	symbol := normalizeSymbol(input.Symbol)
	cooldown := defaultCooldownSec
	if input.CooldownSeconds != nil {
		cooldown = *input.CooldownSeconds
	}

	var created *repoalert.Alert
	_, err := s.db.ConnPool.Transact(ctx, pgx.TxOptions{}, func(ctx context.Context, tx *wpgx.WTx) (any, error) {
		alertRepo := s.db.AlertRepo.WithTx(tx)
		globalCount, err := alertRepo.CountAlerts(ctx)
		if err != nil {
			return nil, err
		}
		if globalCount != nil && *globalCount >= maxAlertsGlobal {
			return nil, mowerror.New(mowerror.FailedPrecondition, "ALERT_LIMIT_GLOBAL_EXCEEDED")
		}
		pairCount, err := alertRepo.CountAlertsByExchangeSymbol(ctx, repoalert.CountAlertsByExchangeSymbolParams{
			Exchange: input.Exchange.String(),
			Symbol:   symbol,
		})
		if err != nil {
			return nil, err
		}
		if pairCount != nil && *pairCount >= maxAlertsPerSymbol {
			return nil, mowerror.New(mowerror.FailedPrecondition, "ALERT_LIMIT_PER_SYMBOL_EXCEEDED")
		}
		created, err = alertRepo.CreateAlert(ctx, repoalert.CreateAlertParams{
			ID:              uuid.NewString(),
			Exchange:        input.Exchange.String(),
			Symbol:          symbol,
			Type:            input.Type,
			Frequency:       repoalert.AlertFrequencyT(input.Frequency),
			Price:           parseNumeric(input.Price),
			AlertWindow:     parseAlertWindow(input.Window),
			Percent:         parseNumeric(input.Percent),
			Remark:          input.Remark,
			CooldownSeconds: int32(cooldown),
		})
		return created, err
	})
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			return nil, mowerror.New(mowerror.AlreadyExists, "ALERT_DUPLICATED")
		}
		return nil, err
	}

	item := toAlertItem(*created)
	s.addRuntimeAlert(item)
	return &item, nil
}

func (s *Service) RemoveAlert(ctx context.Context, id string) (bool, error) {
	alertRow, err := s.db.AlertRepo.GetAlertByID(ctx, id)
	if err != nil {
		return false, err
	}
	if alertRow == nil {
		return false, nil
	}
	affected, err := s.db.AlertRepo.DeleteAlertByID(ctx, id)
	if err != nil {
		return false, err
	}
	if affected <= 0 {
		return false, nil
	}
	s.removeRuntimeAlert(id, watchKey(types.Exchange(alertRow.Exchange), alertRow.Symbol))
	return true, nil
}

func (s *Service) CleanupEventsBefore(ctx context.Context, t time.Time) (int64, error) {
	return s.db.AlertTriggerEventRepo.CleanupAlertTriggerEventsBefore(ctx, t)
}

func (s *Service) addRuntimeAlert(item AlertItem) {
	key := watchKey(item.Exchange, item.Symbol)
	s.mu.Lock()
	s.alerts[item.ID] = newRuntimeAlert(item)
	if _, ok := s.watchers[key]; !ok {
		s.startWatcherLocked(item.Exchange, item.Symbol)
	}
	s.mu.Unlock()
}

func (s *Service) removeRuntimeAlert(id, key string) {
	s.mu.Lock()
	delete(s.alerts, id)
	needStop := true
	for _, a := range s.alerts {
		if watchKey(a.item.Exchange, a.item.Symbol) == key {
			needStop = false
			break
		}
	}
	var w *watcher
	if needStop {
		w = s.watchers[key]
		delete(s.watchers, key)
	}
	s.mu.Unlock()
	if w != nil {
		w.cancel()
	}
}

func (s *Service) startWatcherLocked(exchange types.Exchange, symbol string) {
	if s.cancel == nil {
		s.rootCtx, s.cancel = context.WithCancel(context.Background())
	}
	key := watchKey(exchange, symbol)
	ctx, cancel := context.WithCancel(s.rootCtx)
	w := &watcher{
		exchange: exchange,
		symbol:   symbol,
		cancel:   cancel,
		done:     make(chan struct{}),
	}
	s.watchers[key] = w
	go s.runWatcher(ctx, w)
}

func (s *Service) runWatcher(ctx context.Context, w *watcher) {
	defer close(w.done)
	healthTicker := time.NewTicker(5 * time.Second)
	defer healthTicker.Stop()
	backoff := time.Second
	for {
		// Per-connection context so reconnect / unhealthy exit releases the
		// stream subscription (streamsvc waits on ctx.Done() to ReleaseSubscription).
		subCtx, subCancel := context.WithCancel(ctx)
		req := &types.SubscribeStreamRequest{
			StreamType: types.StreamTypeTicker,
			Exchange:   &w.exchange,
			Symbol:     w.symbol,
		}
		ch, err := s.streamSvc.SubscribeMarketStream(subCtx, req)
		if err != nil {
			subCancel()
			if ctx.Err() != nil {
				return
			}
			log.Error().Err(err).Str("exchange", w.exchange.String()).Str("symbol", w.symbol).Msg("alert watcher subscribe failed")
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = minDuration(backoff*2, 30*time.Second)
			continue
		}
		backoff = time.Second
		lastMsgAt := time.Now()
		var evalBusy atomic.Bool
		lastDispatchedAt := time.Time{}
	recv:
		for {
			select {
			case <-ctx.Done():
				subCancel()
				return
			case <-healthTicker.C:
				if time.Since(lastMsgAt) > 30*time.Second {
					log.Warn().Str("exchange", w.exchange.String()).Str("symbol", w.symbol).Msg("alert watcher unhealthy, reconnecting")
					subCancel()
					break recv
				}
			case msg, ok := <-ch:
				if !ok {
					subCancel()
					break recv
				}
				if msg == nil || msg.Envelope == nil || msg.Envelope.Payload == nil || msg.Envelope.Payload.Ticker == nil {
					continue
				}
				lastMsgAt = time.Now()
				ticker := msg.Envelope.Payload.Ticker
				now := ticker.Ts
				if now.IsZero() {
					now = time.Now()
				}
				price := ticker.LastPrice
				if evalBusy.Load() {
					continue
				}
				dispatchAt := time.Now()
				if !lastDispatchedAt.IsZero() && dispatchAt.Sub(lastDispatchedAt) < time.Second {
					continue
				}
				if !evalBusy.CompareAndSwap(false, true) {
					continue
				}
				lastDispatchedAt = dispatchAt
				nowCopy, priceCopy := now, price
				go func() {
					defer evalBusy.Store(false)
					s.evalAndTrigger(ctx, w, nowCopy, priceCopy)
				}()
			}
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

func (s *Service) evalAndTrigger(ctx context.Context, w *watcher, now time.Time, price decimal.Decimal) {
	key := watchKey(w.exchange, w.symbol)
	s.mu.RLock()
	alerts := make([]*runtimeAlert, 0, 8)
	for _, a := range s.alerts {
		if watchKey(a.item.Exchange, a.item.Symbol) == key {
			alerts = append(alerts, a)
		}
	}
	s.mu.RUnlock()

	for _, ra := range alerts {
		met, baseline, evaluated := ra.evaluate(ctx, s.priceLookup, now, price)
		if !evaluated {
			continue
		}
		if ra.item.Frequency == "repeat" {
			if !met {
				ra.inThreshold = false
				ra.rearmReady = true
				continue
			}
			if ra.inThreshold {
				continue
			}
			ra.inThreshold = true
			if !ra.rearmReady {
				continue
			}
			if ra.item.LastTriggeredAt != nil && now.Sub(*ra.item.LastTriggeredAt) < time.Duration(ra.item.CooldownSeconds)*time.Second {
				continue
			}
			if s.handleTriggered(ctx, ra, now, price, baseline, true) == nil {
				ra.rearmReady = false
			}
			continue
		}
		if met {
			_ = s.handleTriggered(ctx, ra, now, price, baseline, false)
		}
	}
}

func (s *Service) handleTriggered(ctx context.Context, ra *runtimeAlert, now time.Time, triggerPrice decimal.Decimal, baseline *decimal.Decimal, allowRepeat bool) error {
	notifyErr := push.Notify(context.WithoutCancel(ctx), push.NotifyRequest{
		SceneKey: "alarm.price_alert",
		Message:  buildAlertPushMessage(ra.item, triggerPrice, baseline, now),
	})
	notifyResult := "success"
	var errorMessage *string
	if notifyErr != nil {
		notifyResult = "failed"
		msg := notifyErr.Error()
		errorMessage = &msg
	}

	meta, _ := json.Marshal(map[string]any{
		"exchange": ra.item.Exchange.String(),
		"symbol":   ra.item.Symbol,
		"type":     ra.item.Type,
	})
	params := repoevent.CreateAlertTriggerEventParams{
		ID:            uuid.NewString(),
		AlertID:       ra.item.ID,
		Exchange:      ra.item.Exchange.String(),
		Symbol:        ra.item.Symbol,
		Type:          ra.item.Type,
		Frequency:     ra.item.Frequency,
		TargetPrice:   parseNumeric(decimalPtrToStringPtr(ra.item.Price)),
		AlertWindow:   ra.item.Window,
		Percent:       parseNumeric(decimalPtrToStringPtr(ra.item.Percent)),
		BaselinePrice: parseNumeric(decimalPtrToStringPtr(baseline)),
		TriggerPrice:  utils.Decimal.DecimalToPgNumeric(triggerPrice),
		TriggeredAt:   now,
		NotifyResult:  notifyResult,
		ErrorMessage:  errorMessage,
		Meta:          meta,
	}
	if _, err := s.db.AlertTriggerEventRepo.CreateAlertTriggerEvent(ctx, params); err != nil {
		return err
	}
	if err := s.db.AlertRepo.TouchAlertTriggered(ctx, repoalert.TouchAlertTriggeredParams{
		ID:              ra.item.ID,
		LastTriggeredAt: &now,
	}); err != nil {
		return err
	}
	ra.item.LastTriggeredAt = &now
	ra.item.TriggerCount++

	log.Info().
		Str("alert_id", ra.item.ID).
		Str("exchange", ra.item.Exchange.String()).
		Str("symbol", ra.item.Symbol).
		Str("type", ra.item.Type).
		Str("trigger_price", triggerPrice.String()).
		Str("notify_result", notifyResult).
		Msg("alert triggered")

	if ra.item.Frequency == "once" {
		_, _ = s.db.AlertRepo.DeleteAlertByID(ctx, ra.item.ID)
		s.removeRuntimeAlert(ra.item.ID, watchKey(ra.item.Exchange, ra.item.Symbol))
	} else if !allowRepeat {
		ra.rearmReady = false
	}
	return nil
}

func buildAlertPushMessage(item AlertItem, triggerPrice decimal.Decimal, baseline *decimal.Decimal, now time.Time) string {
	_ = triggerPrice
	_ = baseline
	_ = now

	parts := []string{
		fmt.Sprintf("🔔 %s:%s 价格预警", strings.ToUpper(item.Exchange.String()), item.Symbol),
		buildAlertSummary(item),
	}
	if item.Remark != nil && strings.TrimSpace(*item.Remark) != "" {
		parts = append(parts, fmt.Sprintf("备注: %s", strings.TrimSpace(*item.Remark)))
	}
	return strings.Join(parts, "\n")
}

func buildAlertSummary(item AlertItem) string {
	switch item.Type {
	case "price_reach":
		if item.Price != nil {
			return fmt.Sprintf("价格达到 %s", formatPriceForAlert(*item.Price))
		}
	case "price_rise_to":
		if item.Price != nil {
			return fmt.Sprintf("价格上涨至 %s", formatPriceForAlert(*item.Price))
		}
	case "price_fall_to":
		if item.Price != nil {
			return fmt.Sprintf("价格下跌至 %s", formatPriceForAlert(*item.Price))
		}
	case "price_rise_pct_over":
		if item.Window != nil && item.Percent != nil {
			return fmt.Sprintf("价格 %s内 涨幅达 %s%%", *item.Window, item.Percent.String())
		}
	case "price_fall_pct_over":
		if item.Window != nil && item.Percent != nil {
			return fmt.Sprintf("价格 %s内 跌幅达 %s%%", *item.Window, item.Percent.String())
		}
	}
	return "价格预警触发"
}

func formatPriceForAlert(v decimal.Decimal) string {
	raw := v.String()
	sign := ""
	if strings.HasPrefix(raw, "-") {
		sign = "-"
		raw = strings.TrimPrefix(raw, "-")
	}
	parts := strings.SplitN(raw, ".", 2)
	intPart := parts[0]
	if intPart == "" {
		intPart = "0"
	}
	var groups []string
	for len(intPart) > 3 {
		groups = append([]string{intPart[len(intPart)-3:]}, groups...)
		intPart = intPart[:len(intPart)-3]
	}
	groups = append([]string{intPart}, groups...)
	formatted := sign + strings.Join(groups, ",")
	if len(parts) == 2 && parts[1] != "" {
		formatted += "." + parts[1]
	}
	return formatted
}

func validateExchangeSymbol(exchange types.Exchange, symbol string) error {
	if !exchange.IsValid() {
		return mowerror.New(mowerror.InvalidArgument, fmt.Sprintf("invalid exchange: %s", exchange))
	}
	if strings.TrimSpace(symbol) == "" {
		return mowerror.New(mowerror.InvalidArgument, "symbol is required")
	}
	return nil
}

func validateAddInput(input AddAlertInput) error {
	if err := validateExchangeSymbol(input.Exchange, input.Symbol); err != nil {
		return err
	}
	if input.Frequency != "repeat" && input.Frequency != "once" {
		return mowerror.New(mowerror.InvalidArgument, "ALERT_INVALID_FIELD_COMBINATION")
	}
	switch input.Type {
	case "price_reach", "price_rise_to", "price_fall_to":
		if input.Price == nil || strings.TrimSpace(*input.Price) == "" {
			return mowerror.New(mowerror.InvalidArgument, "ALERT_INVALID_FIELD_COMBINATION")
		}
		if input.Window != nil || input.Percent != nil {
			return mowerror.New(mowerror.InvalidArgument, "ALERT_INVALID_FIELD_COMBINATION")
		}
		d, err := decimal.NewFromString(strings.TrimSpace(*input.Price))
		if err != nil || !d.GreaterThan(decimal.Zero) {
			return mowerror.New(mowerror.InvalidArgument, "ALERT_INVALID_FIELD_COMBINATION")
		}
	case "price_rise_pct_over", "price_fall_pct_over":
		if input.Price != nil || input.Window == nil || input.Percent == nil {
			return mowerror.New(mowerror.InvalidArgument, "ALERT_INVALID_FIELD_COMBINATION")
		}
		if _, ok := alertWindowToDuration(*input.Window); !ok {
			return mowerror.New(mowerror.InvalidArgument, "ALERT_INVALID_FIELD_COMBINATION")
		}
		d, err := decimal.NewFromString(strings.TrimSpace(*input.Percent))
		if err != nil || !d.GreaterThan(decimal.Zero) {
			return mowerror.New(mowerror.InvalidArgument, "ALERT_INVALID_FIELD_COMBINATION")
		}
	default:
		return mowerror.New(mowerror.InvalidArgument, "ALERT_INVALID_FIELD_COMBINATION")
	}
	if input.CooldownSeconds != nil && *input.CooldownSeconds < 0 {
		return mowerror.New(mowerror.InvalidArgument, "cooldownSeconds must be >= 0")
	}
	return nil
}

func toAlertItem(row repoalert.Alert) AlertItem {
	var price *decimal.Decimal
	if row.Price.Valid {
		p := utils.Decimal.PgNumericToDecimal(row.Price)
		price = &p
	}
	var percent *decimal.Decimal
	if row.Percent.Valid {
		p := utils.Decimal.PgNumericToDecimal(row.Percent)
		percent = &p
	}
	return AlertItem{
		ID:              row.ID,
		Exchange:        types.Exchange(row.Exchange),
		Symbol:          row.Symbol,
		Type:            row.Type,
		Frequency:       string(row.Frequency),
		Price:           price,
		Window:          nullAlertWindowToPtr(row.AlertWindow),
		Percent:         percent,
		Remark:          row.Remark,
		CooldownSeconds: int(row.CooldownSeconds),
		Status:          string(row.Status),
		LastTriggeredAt: row.LastTriggeredAt,
		TriggerCount:    int(row.TriggerCount),
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

func parseNumeric(v *string) pgtype.Numeric {
	if v == nil || strings.TrimSpace(*v) == "" {
		return pgtype.Numeric{}
	}
	d, err := decimal.NewFromString(strings.TrimSpace(*v))
	if err != nil {
		return pgtype.Numeric{}
	}
	return utils.Decimal.DecimalToPgNumeric(d)
}

func parseAlertWindow(v *string) repoalert.NullAlertWindowT {
	if v == nil || strings.TrimSpace(*v) == "" {
		return repoalert.NullAlertWindowT{}
	}
	return repoalert.NullAlertWindowT{
		AlertWindowT: repoalert.AlertWindowT(strings.TrimSpace(*v)),
		Valid:        true,
	}
}

func nullAlertWindowToPtr(v repoalert.NullAlertWindowT) *string {
	if !v.Valid {
		return nil
	}
	s := string(v.AlertWindowT)
	return &s
}

func decimalPtrToStringPtr(v *decimal.Decimal) *string {
	if v == nil {
		return nil
	}
	s := v.String()
	return &s
}

func normalizeSymbol(symbol string) string {
	return strings.ToUpper(strings.TrimSpace(symbol))
}

func watchKey(exchange types.Exchange, symbol string) string {
	return fmt.Sprintf("%s|%s", exchange.String(), normalizeSymbol(symbol))
}

func alertWindowToDuration(v string) (time.Duration, bool) {
	switch v {
	case "5m":
		return 5 * time.Minute, true
	case "1h":
		return time.Hour, true
	case "4h":
		return 4 * time.Hour, true
	case "24h":
		return 24 * time.Hour, true
	default:
		return 0, false
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
