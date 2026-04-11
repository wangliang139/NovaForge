package account

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bytedance/sonic"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/internal/rstream"
	"github.com/wangliang139/NovaForge/server/pkg/repos/ledgers"
	"github.com/wangliang139/NovaForge/server/pkg/repos/positions"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/utils"
	"github.com/wangliang139/mow/logger"
	"github.com/wangliang139/mow/snowflake"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const TracerName = "github.com/wangliang139/NovaForge/server/pkg/entity/account"

var tracer = otel.Tracer(TracerName)

func (e *Entity) Start() error {
	uuid := snowflake.Generate().String()
	go e.ListenAccountEvent(uuid)
	return nil
}

func (e *Entity) Stop() error {
	e.cancelFunc()
	return nil
}

func (e *Entity) ListenAccountEvent(consumerId string) {
	streamKey := e.cfg.AccountRawMsgTopic
	group := e.cfg.AccountConsumerGroup

	ch := rstream.Subscribe(e.ctx, e.cache, streamKey, group, consumerId)
	for {
		select {
		case <-e.ctx.Done():
			return
		case payload := <-ch:
			var envelope ctypes.Envelope
			if err := sonic.Unmarshal(payload, &envelope); err != nil {
				log.Err(err).Str("consumer_id", consumerId).Msg("Failed to unmarshal account message")
				continue
			}

			var err error
			ctx := context.Background()
			ctx, span := tracer.Start(ctx, "account.consume")
			defer func() {
				span.SetAttributes(attribute.String("exchange", envelope.Exchange))
				if envelope.Account != nil {
					span.SetAttributes(attribute.String("account", *envelope.Account))
				}
				if envelope.Symbol != nil {
					span.SetAttributes(attribute.String("symbol", *envelope.Symbol))
				}
				span.SetAttributes(attribute.String("stream", envelope.Stream.String()))
				if err != nil {
					span.SetStatus(codes.Error, err.Error())
				} else {
					span.SetStatus(codes.Ok, "success")
				}
				span.End()
			}()

			logger.Ctx(ctx).Info().Str("exchange", envelope.Exchange).Any("account", envelope.Account).Interface("message", envelope.Payload).Msg("Receive account message")
			if envelope.Account == nil {
				logger.Ctx(ctx).Err(errors.New("account id is required")).Str("consumer_id", consumerId).Msg("Failed to process account message")
				continue
			}
			err = e.handleAccountMessage(ctx, &envelope)
			if err != nil {
				logger.Ctx(ctx).Err(err).Str("consumer_id", consumerId).Msg("Failed to process account message")
				continue
			}
		}
	}
}

// HandleAccountMessage 将账户相关的市场消息落库为快照与资金流水
func (e *Entity) handleAccountMessage(ctx context.Context, envelope *ctypes.Envelope) error {
	if envelope == nil {
		return nil
	}
	if envelope.Payload == nil {
		return nil
	}
	accountID := *envelope.Account
	exchange := ctypes.Exchange(envelope.Exchange)
	if envelope.Payload.BalanceSnapshot != nil {
		return e.handleAssetSnapshot(ctx, accountID, exchange, envelope.Payload.BalanceSnapshot)
	}
	if envelope.Payload.BalanceUpdate != nil {
		return e.HandleAssetUpdates(ctx, accountID, exchange, envelope.Payload.BalanceUpdate)
	}
	if envelope.Payload.PositionSnapshot != nil {
		return e.handlePositionsSnapshot(ctx, accountID, exchange, envelope.Payload.PositionSnapshot.Positions, true)
	}
	if envelope.Payload.PositionsUpdate != nil {
		return e.handlePositionsSnapshot(ctx, accountID, exchange, envelope.Payload.PositionsUpdate.Positions, false)
	}
	if envelope.Payload.Order != nil {
		return e.handleOrderUpdate(ctx, accountID, exchange, envelope.Payload.Order)
	}
	if envelope.Payload.SymbolLeverage != nil {
		return e.handleSymbolLeverageUpdate(ctx, accountID, exchange, envelope.Payload.SymbolLeverage)
	}
	return nil
}

func (e *Entity) handleAssetSnapshot(ctx context.Context, accountID string, exchange ctypes.Exchange, snapshot *ctypes.BalanceSnapshot) error {
	if snapshot == nil || len(snapshot.Assets) == 0 {
		return nil
	}

	balance := &ctypes.Balance{
		Assets: make([]*ctypes.AssetBo, 0, len(snapshot.Assets)),
	}
	for _, asset := range snapshot.Assets {
		if asset == nil {
			continue
		}
		balance.Assets = append(balance.Assets, &ctypes.AssetBo{
			WalletType: asset.WalletType,
			Code:       asset.Code,
			Balance:    *asset.Balance,
			Locked:     *asset.Locked,
			UpdatedTs:  asset.UpdatedTs,
		})
	}
	return e.ApplyAccountBalance(ctx, accountID, exchange, snapshot.Scope, balance)
}

func (e *Entity) HandleAssetUpdates(ctx context.Context, accountID string, exchange ctypes.Exchange, update *ctypes.BalanceUpdate) error {
	if update == nil || len(update.Assets) == 0 {
		return nil
	}

	exchangeStr := exchange.String()
	for _, asset := range update.Assets {
		if asset == nil {
			continue
		}
		walletType := asset.WalletType
		if !walletType.Valid() {
			walletType = ctypes.WalletTypeFund
		}

		ts := time.Now()
		if !asset.UpdatedTs.IsZero() {
			ts = asset.UpdatedTs
		}

		ledgerParams := ledgers.CreateLedgerEntryParams{
			AccountID:   accountID,
			Exchange:    exchangeStr,
			Asset:       asset.Code,
			WalletType:  ledgers.WalletType(walletType),
			Type:        string(update.Reason),
			Detail:      update.Detail,
			Ts:          ts,
			IsEffective: false,
		}

		if update.Type == ctypes.UpdateTypeSnapshot {
			if asset.Balance == nil && asset.Locked == nil {
				logger.Ctx(ctx).Err(errors.New("balance and locked are nil")).
					Str("account_id", accountID).
					Str("exchange", exchangeStr).
					Str("asset", asset.Code).
					Str("type", string(walletType)).
					Msg("failed to apply asset update")
				continue
			}
			total := asset.Balance
			frozen := asset.Locked
			// 币安资产的冻结/解冻由订单快照事件推导而来，无需重复落库
			if exchange.Base() == ctypes.ExchangeBinance {
				frozen = lo.ToPtr(decimal.Zero)
			}
			row, err := e.ApplyAssetSnapshot(ctx, accountID, exchange, walletType, asset.Code, total, frozen, ts)
			if err != nil {
				logger.Ctx(ctx).Err(err).
					Str("account_id", accountID).
					Str("exchange", exchangeStr).
					Str("asset", asset.Code).
					Str("type", string(walletType)).
					Msg("failed to apply asset snapshot")
				continue
			}
			if total != nil {
				ledgerParams.Total = utils.Decimal.DecimalToPgNumeric(*total)
			}
			if frozen != nil {
				ledgerParams.Frozen = utils.Decimal.DecimalToPgNumeric(*frozen)
			}
			if row != nil {
				prevTotal := utils.Decimal.PgNumericToDecimal(row.PrevTotal)
				prevFrozen := utils.Decimal.PgNumericToDecimal(row.PrevFrozen)
				total := utils.Decimal.PgNumericToDecimal(row.Total)
				frozen := utils.Decimal.PgNumericToDecimal(row.Frozen)
				totalDelta := total.Sub(prevTotal)
				frozenDelta := frozen.Sub(prevFrozen)
				ledgerParams.TotalDelta = utils.Decimal.DecimalToPgNumeric(totalDelta)
				ledgerParams.FrozenDelta = utils.Decimal.DecimalToPgNumeric(frozenDelta)
				ledgerParams.IsEffective = true
				_totalDelta := totalDelta.String()
				_frozenDelta := frozenDelta.String()
				_ = _totalDelta
				_ = _frozenDelta
				if totalDelta.Abs().LessThan(MinDelta) && frozenDelta.Abs().LessThan(MinDelta) {
					continue
				}

				// 将快照语义转换为增量语义对外发布（避免下游把 snapshot 当 delta 使用）
				outUpdate := &ctypes.BalanceUpdate{
					Type:   ctypes.UpdateTypeIncrement,
					Reason: update.Reason,
					Assets: []*ctypes.AssetEvent{
						{
							WalletType: walletType,
							Code:       asset.Code,
							Balance:    lo.ToPtr(totalDelta),
							Locked:     lo.ToPtr(frozenDelta),
							UpdatedTs:  ts,
						},
					},
					Detail: update.Detail,
				}
				selector := ctypes.StreamSelector{
					Stream:  ctypes.StreamTypeAccount,
					Account: lo.ToPtr(accountID),
				}
				msg := ctypes.NewMessage(exchange, selector, outUpdate, ts)
				if e.engine != nil {
					if err := e.engine.Publish(ctx, msg); err != nil {
						return err
					}
				}
			}
		} else {
			if (asset.Balance == nil || asset.Balance.IsZero()) && (asset.Locked == nil || asset.Locked.IsZero()) {
				logger.Ctx(ctx).Err(errors.New("balance and locked are zero")).
					Str("account_id", accountID).
					Str("exchange", exchangeStr).
					Str("asset", asset.Code).
					Str("type", string(walletType)).
					Msg("failed to apply asset update")
				continue
			}
			totalDelta := asset.Balance
			frozenDelta := asset.Locked
			// 币安资产的冻结/解冻由订单快照事件推导而来，无需重复落库
			if exchange.Base() == ctypes.ExchangeBinance {
				frozenDelta = lo.ToPtr(decimal.Zero)
			}
			assetPo, err := e.ApplyAssetIncrement(ctx, accountID, exchange, walletType, asset.Code, totalDelta, frozenDelta, ts)
			if err != nil {
				logger.Ctx(ctx).Err(err).
					Str("account_id", accountID).
					Str("exchange", exchangeStr).
					Str("asset", asset.Code).
					Str("type", string(walletType)).
					Msg("failed to apply asset update")
				continue
			}

			if totalDelta != nil {
				ledgerParams.TotalDelta = utils.Decimal.DecimalToPgNumeric(*totalDelta)
			} else {
				ledgerParams.TotalDelta = utils.Decimal.DecimalToPgNumeric(decimal.Zero)
			}
			if frozenDelta != nil {
				ledgerParams.FrozenDelta = utils.Decimal.DecimalToPgNumeric(*frozenDelta)
			} else {
				ledgerParams.FrozenDelta = utils.Decimal.DecimalToPgNumeric(decimal.Zero)
			}
			if assetPo != nil {
				ledgerParams.Total = assetPo.Total
				ledgerParams.Frozen = assetPo.Frozen
				ledgerParams.IsEffective = true
			}
			// 增量语义直接对外发布（Balance/Locked 均为 delta）
			td := decimal.Zero
			fd := decimal.Zero
			if totalDelta != nil {
				td = *totalDelta
			}
			if frozenDelta != nil {
				fd = *frozenDelta
			}
			outUpdate := &ctypes.BalanceUpdate{
				Type:   ctypes.UpdateTypeIncrement,
				Reason: update.Reason,
				Assets: []*ctypes.AssetEvent{
					{
						WalletType: walletType,
						Code:       asset.Code,
						Balance:    lo.ToPtr(td),
						Locked:     lo.ToPtr(fd),
						UpdatedTs:  ts,
					},
				},
				Detail: update.Detail,
			}
			selector := ctypes.StreamSelector{
				Stream:  ctypes.StreamTypeAccount,
				Account: lo.ToPtr(accountID),
			}
			msg := ctypes.NewMessage(exchange, selector, outUpdate, ts)
			if e.engine != nil {
				if err := e.engine.Publish(ctx, msg); err != nil {
					return err
				}
			}
		}

		go func() {
			ctx := context.WithoutCancel(ctx)
			if err := e.AppendLedger(ctx, ledgerParams); err != nil {
				logger.Ctx(ctx).Err(err).Str("account_id", accountID).
					Str("exchange", exchangeStr).
					Str("asset", asset.Code).
					Str("type", string(walletType)).
					Msg("failed to append ledger entry")
			}
		}()
	}
	return nil
}

func (e *Entity) handlePositionsSnapshot(ctx context.Context, accountID string, exchange ctypes.Exchange, positions []*ctypes.Position, closeOthers bool) error {
	if positions == nil {
		return nil
	}
	return e.ApplyAccountPositions(ctx, accountID, exchange, positions, closeOthers)
}

func (e *Entity) handleSymbolLeverageUpdate(ctx context.Context, accountID string, exchange ctypes.Exchange, update *ctypes.SymbolLeverage) error {
	if update == nil {
		return nil
	}
	exchangeStr := exchange.String()

	symbol := update.Symbol.String()
	leverage := update.Leverage

	_, err := e.db.PositionsRepo.UpsertSymbolLeverage(ctx, positions.UpsertSymbolLeverageParams{
		AccountID: accountID,
		Exchange:  exchangeStr,
		Symbol:    symbol,
		Side:      positions.PositionSide(update.Side),
		Leverage:  int32(leverage),
		UpdatedTs: time.Now(),
	})
	if err != nil {
		return fmt.Errorf("apply symbol leverage update: %w", err)
	}

	// 处理后发布到事件总线（供下游订阅）
	ts := time.Now()
	if !update.UpdatedTs.IsZero() {
		ts = update.UpdatedTs
	}
	selector := ctypes.StreamSelector{
		Stream:  ctypes.StreamTypeAccount,
		Account: lo.ToPtr(accountID),
	}
	msg := ctypes.NewMessage(exchange, selector, update, ts)
	if e.engine != nil {
		if err := e.engine.Publish(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

func (e *Entity) handleOrderUpdate(ctx context.Context, accountID string, exchange ctypes.Exchange, ord *ctypes.Order) error {
	if ord == nil {
		return errors.New("order is nil")
	}
	ord.AccountID = accountID
	ord.Exchange = exchange

	// 落库并派生成交/冻结事件
	prevOrder, err := e.ApplyOrderSnapshot(ctx, ord)
	if err != nil {
		return err
	}
	e.maybeNotifyOrderTelegram(ctx, prevOrder, ord)

	// 发布订单快照事件
	ts := time.Now()
	if !ord.UpdatedTs.IsZero() {
		ts = ord.UpdatedTs
	}
	selector := ctypes.StreamSelector{
		Stream:  ctypes.StreamTypeAccount,
		Account: lo.ToPtr(accountID),
	}
	msg := ctypes.NewMessage(exchange, selector, ord, ts)
	if e.engine != nil {
		if err := e.engine.Publish(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}
