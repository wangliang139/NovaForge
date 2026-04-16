// Command audit：virtual_sub 现货订单资金稽核（按订单当前行累计成交量/成交额/手续费一次性套现货公式）。
//
// 单订单：expected = asset_snapshot.total(<= created_ts) + 理论增量；actual = effective_ts <= updated_ts 的最近一条快照 total。
// 多订单：expected = asset_snapshot.total(<= start_ts) + Σ(各单理论增量)；actual = end_ts 时点快照 total。
//
// 注意：asset_snapshot.frozen 已删除，脚本仅比较 total，不参与 frozen 对账。
// 注意：单订单若生命周期内存在其它资金变动，或多次部分成交与「整单一次性」舍入不一致，可能出现偏差；请用 range 或逐笔 ledger 排查。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/repos"
	"github.com/wangliang139/NovaForge/server/pkg/repos/account"
	"github.com/wangliang139/NovaForge/server/pkg/repos/acct_snapshot"
	"github.com/wangliang139/NovaForge/server/pkg/repos/orders"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/utils"
	"github.com/wangliang139/mow/database/cache"
	"github.com/wangliang139/mow/database/wpgx"
)

type assetKey struct {
	wt   acct_snapshot.WalletType
	code string
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	ctx := context.Background()
	switch os.Args[1] {
	case "single":
		runSingle(ctx, os.Args[2:])
	case "range":
		runRange(ctx, os.Args[2:])
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n", os.Args[1])
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `usage:
  audit single  -account <id> -order-id <id> [-tol <decimal>] [-html-out <path>] [-no-browser]
  audit range   -account <id> -symbol <sym> -start <RFC3339> -end <RFC3339> [-tol <decimal>] [-html-out <path>] [-no-browser]

默认在临时目录生成 HTML 并用系统浏览器打开；-no-browser 仅写文件；-html-out 指定输出路径。

环境：与主进程相同，需可连接 postgres（wpgx NewWPGXPool "postgres"）。
`)
}

func openRepos(ctx context.Context) (*repos.Entity, func(), error) {
	os.Setenv("POSTGRES_HOST", "localhost")
	os.Setenv("POSTGRES_PASSWORD", "my-secret")
	os.Setenv("POSTGRES_APPNAME", "novaforge")
	os.Setenv("POSTGRES_DBNAME", "llt_data_db")
	pool, err := wpgx.NewWPGXPool(ctx, "postgres")
	if err != nil {
		return nil, nil, fmt.Errorf("db pool: %w", err)
	}
	redisCli := cache.NewRedisClient("redis")
	dCache, err := cache.NewDCache("dcache", redisCli)
	if err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("dcache: %w", err)
	}
	db := repos.New(pool, dCache)
	closer := func() {
		pool.Close()
		_ = redisCli.Close()
	}
	return db, closer, nil
}

func runSingle(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("single", flag.ExitOnError)
	accountID := fs.String("account", "", "virtual_sub account id")
	orderID := fs.String("order-id", "", "order id (exchange order id)")
	tolStr := fs.String("tol", "0", "absolute tolerance per asset (decimal)")
	htmlOut := fs.String("html-out", "", "write HTML report to this path (default: temp file)")
	noBrowser := fs.Bool("no-browser", false, "do not open HTML in browser")
	_ = fs.Parse(args)

	if strings.TrimSpace(*accountID) == "" || strings.TrimSpace(*orderID) == "" {
		fs.Usage()
		os.Exit(2)
	}
	tol, err := decimal.NewFromString(*tolStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid -tol: %v\n", err)
		os.Exit(2)
	}

	db, closer, err := openRepos(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer closer()

	acct, err := db.AccountRepo.GetById(ctx, *accountID)
	if err != nil || acct == nil {
		fmt.Fprintf(os.Stderr, "account: %v\n", err)
		os.Exit(1)
	}
	if acct.AccountType != account.AccountTypeVirtualSub {
		fmt.Fprintf(os.Stderr, "account %s is not virtual_sub\n", *accountID)
		os.Exit(1)
	}

	ex := ctypes.Exchange(acct.Exchange)
	if !ex.IsValid() {
		fmt.Fprintf(os.Stderr, "invalid account exchange %s\n", acct.Exchange)
		os.Exit(1)
	}
	exStr := string(acct.Exchange)

	ord, err := db.OrdersRepo.GetOrderByOrderId(ctx, orders.GetOrderByOrderIdParams{
		AccountID: *accountID,
		OrderID:   strings.TrimSpace(*orderID),
	})
	if err != nil || ord == nil {
		fmt.Fprintf(os.Stderr, "order: %v\n", err)
		os.Exit(1)
	}
	if ord.Exchange != exStr {
		fmt.Fprintf(os.Stderr, "order.exchange %q != account.exchange %q\n", ord.Exchange, exStr)
		os.Exit(1)
	}

	if err := validateSpotOrderForAudit(ord); err != nil {
		fmt.Fprintf(os.Stderr, "order not eligible: %v\n", err)
		os.Exit(1)
	}

	delta, err := spotOrderTotalDelta(ord, ex)
	if err != nil {
		fmt.Fprintf(os.Stderr, "delta: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("mode=single account=%s order_id=%s symbol=%s created_ts=%s updated_ts=%s\n",
		*accountID, ord.OrderID, ord.Symbol, ord.CreatedTs.Format(time.RFC3339Nano), ord.UpdatedTs.Format(time.RFC3339Nano))
	fmt.Printf("note: expected uses baseline snapshot at or before created_ts; if other activity overlapped, use range mode.\n\n")

	rep := &htmlReport{
		GeneratedAt: time.Now(),
		Mode:        "single",
		AccountID:   *accountID,
		Tolerance:   tol.String(),
		MetaLines: []string{
			fmt.Sprintf("order_id=%s symbol=%s", ord.OrderID, ord.Symbol),
			"预期：created_ts 基线 total + 整单理论增量；实际：effective_ts ≤ updated_ts 的最近一条 asset_snapshot.total",
			"不比较 frozen（字段已删除）",
		},
	}

	exit := 0
	keys := sortedAssetKeys(delta)
	for _, k := range keys {
		d := delta[k]
		if d.IsZero() {
			continue
		}
		baseRow, err := db.AcctSnapshotRepo.GetAccountAssetSnapshotAtOrBefore(ctx, acct_snapshot.GetAccountAssetSnapshotAtOrBeforeParams{
			AccountID:   *accountID,
			Exchange:    exStr,
			Asset:       k.code,
			WalletType:  k.wt,
			EffectiveTs: ord.CreatedTs,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "baseline snap %s/%s: %v\n", k.wt, k.code, err)
			os.Exit(1)
		}
		baseTotal := decimal.Zero
		if baseRow != nil {
			baseTotal = utils.Decimal.PgNumericToDecimal(baseRow.Total)
		}
		exp := baseTotal.Add(d)

		snapAt, err := db.AcctSnapshotRepo.GetAccountAssetSnapshotAtOrAfter(ctx, acct_snapshot.GetAccountAssetSnapshotAtOrAfterParams{
			AccountID:   *accountID,
			Exchange:    exStr,
			Asset:       k.code,
			WalletType:  k.wt,
			EffectiveTs: ord.UpdatedAt,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "snapshot at updated_ts %s/%s: %v\n", k.wt, k.code, err)
			os.Exit(1)
		}
		row := assetCompareRow{
			Asset:     k.code,
			Wallet:    string(k.wt),
			BaseTotal: baseTotal.String(),
			Delta:     d.String(),
			Expected:  exp.String(),
			SnapRef:   "",
		}
		if snapAt == nil {
			row.Actual = "—"
			row.Diff = "—"
			row.Status = "MISSING"
			row.SnapRef = "updated_ts 及之前无快照"
			exit = 1
			fmt.Printf("asset=%s wallet=%s MISSING_SNAPSHOT_AT_OR_BEFORE updated_ts\n", k.code, k.wt)
		} else {
			act := utils.Decimal.PgNumericToDecimal(snapAt.Total)
			diff := act.Sub(exp)
			ok := diff.Abs().LessThanOrEqual(tol)
			st := "OK"
			if !ok {
				st = "MISMATCH"
				exit = 1
			}
			row.Actual = act.String()
			row.Diff = diff.String()
			row.Status = st
			row.SnapRef = fmt.Sprintf("≤updated_ts 最近快照 effective_ts=%s id=%d", snapAt.EffectiveTs.Format(time.RFC3339Nano), snapAt.ID)
			fmt.Printf("asset=%s wallet=%s base_total=%s delta_total=%s expected_total=%s actual_total=%s diff_total=%s %s (snap_effective_ts=%s)\n",
				k.code, k.wt, baseTotal.String(), d.String(), exp.String(), act.String(), diff.String(), st,
				snapAt.EffectiveTs.Format(time.RFC3339Nano))
		}
		rep.AssetRows = append(rep.AssetRows, row)
	}

	htmlPath := strings.TrimSpace(*htmlOut)
	if htmlPath == "" {
		var err error
		htmlPath, err = defaultReportPath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "report path: %v\n", err)
			os.Exit(1)
		}
	}
	rep.HTMLPath = htmlPath
	rep.ExitCode = exit
	if err := writeHTMLReport(htmlPath, rep); err != nil {
		fmt.Fprintf(os.Stderr, "write html: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\nHTML 报告: %s\n", htmlPath)
	if !*noBrowser {
		if err := openInBrowser(htmlPath); err != nil {
			fmt.Fprintf(os.Stderr, "open browser: %v\n", err)
		}
	}
	os.Exit(exit)
}

func runRange(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("range", flag.ExitOnError)
	accountID := fs.String("account", "", "virtual_sub account id")
	symbol := fs.String("symbol", "", "symbol as stored in orders.symbol, e.g. BTC/USDT:SPOT")
	startS := fs.String("start", "", "window start RFC3339 (filter updated_ts >= start)")
	endS := fs.String("end", "", "window end RFC3339 (filter updated_ts <= end)")
	tolStr := fs.String("tol", "0", "absolute tolerance per asset (decimal)")
	htmlOut := fs.String("html-out", "", "write HTML report to this path (default: temp file)")
	noBrowser := fs.Bool("no-browser", false, "do not open HTML in browser")
	_ = fs.Parse(args)

	if strings.TrimSpace(*accountID) == "" || strings.TrimSpace(*symbol) == "" || *startS == "" || *endS == "" {
		fs.Usage()
		os.Exit(2)
	}
	start, err := time.Parse(time.RFC3339, *startS)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start: %v\n", err)
		os.Exit(2)
	}
	end, err := time.Parse(time.RFC3339, *endS)
	if err != nil {
		fmt.Fprintf(os.Stderr, "end: %v\n", err)
		os.Exit(2)
	}
	if !end.After(start) {
		fmt.Fprintf(os.Stderr, "end must be after start\n")
		os.Exit(2)
	}
	tol, err := decimal.NewFromString(*tolStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid -tol: %v\n", err)
		os.Exit(2)
	}

	db, closer, err := openRepos(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer closer()

	acct, err := db.AccountRepo.GetById(ctx, *accountID)
	if err != nil || acct == nil {
		fmt.Fprintf(os.Stderr, "account: %v\n", err)
		os.Exit(1)
	}
	if acct.AccountType != account.AccountTypeVirtualSub {
		fmt.Fprintf(os.Stderr, "account %s is not virtual_sub\n", *accountID)
		os.Exit(1)
	}
	ex := ctypes.Exchange(acct.Exchange)
	if !ex.IsValid() {
		fmt.Fprintf(os.Stderr, "invalid account exchange %s\n", acct.Exchange)
		os.Exit(1)
	}
	exStr := string(acct.Exchange)

	orderRows, err := listSpotOrdersInUpdatedRange(ctx, db, *accountID, exStr, *symbol, start, end)
	if err != nil {
		fmt.Fprintf(os.Stderr, "list orders: %v\n", err)
		os.Exit(1)
	}

	sumDelta := make(map[assetKey]decimal.Decimal)
	fmt.Printf("mode=range account=%s symbol=%s start=%s end=%s orders=%d\n\n",
		*accountID, *symbol, start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano), len(orderRows))

	rep := &htmlReport{
		GeneratedAt: time.Now(),
		Mode:        "range",
		AccountID:   *accountID,
		Tolerance:   tol.String(),
		MetaLines: []string{
			fmt.Sprintf("symbol=%s window=[%s .. %s]", *symbol, start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano)),
			fmt.Sprintf("区间内订单行数=%d（计入 sum 的需通过 DONE/PARTIAL_DONE + 现货校验）", len(orderRows)),
		},
	}

	for i := range orderRows {
		ord := orderRows[i]
		orow := orderAuditRow{
			OrderID:   ord.OrderID,
			UpdatedTs: ord.UpdatedTs.Format(time.RFC3339Nano),
			IsBuy:     ord.IsBuy,
			ExecQty:   utils.Decimal.PgNumericToDecimal(ord.ExecutedQty).String(),
			ExecQuote: utils.Decimal.PgNumericToDecimal(ord.ExecutedPrice).String(),
			Fee:       utils.Decimal.PgNumericToDecimal(ord.Fee).String(),
			FeeAsset:  strPtr(ord.FeeAsset),
		}
		if err := validateSpotOrderForAudit(&ord); err != nil {
			orow.SkipReason = err.Error()
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("跳过订单 %s: %v", ord.OrderID, err))
			fmt.Fprintf(os.Stderr, "skip order %s: %v\n", ord.OrderID, err)
			rep.OrderRows = append(rep.OrderRows, orow)
			continue
		}
		d, err := spotOrderTotalDelta(&ord, ex)
		if err != nil {
			fmt.Fprintf(os.Stderr, "order %s delta: %v\n", ord.OrderID, err)
			os.Exit(1)
		}
		fmt.Printf("order=%s updated=%s buy=%v exec_qty=%s exec_quote=%s fee=%s fee_asset=%v\n",
			ord.OrderID, ord.UpdatedTs.Format(time.RFC3339Nano), ord.IsBuy,
			utils.Decimal.PgNumericToDecimal(ord.ExecutedQty).String(),
			utils.Decimal.PgNumericToDecimal(ord.ExecutedPrice).String(),
			utils.Decimal.PgNumericToDecimal(ord.Fee).String(), strPtr(ord.FeeAsset))
		for k, v := range d {
			sumDelta[k] = sumDelta[k].Add(v)
		}
		rep.OrderRows = append(rep.OrderRows, orow)
	}

	exit := 0
	for _, k := range sortedAssetKeys(sumDelta) {
		d := sumDelta[k]
		if d.IsZero() {
			continue
		}
		baseRow, err := db.AcctSnapshotRepo.GetAccountAssetSnapshotAtOrBefore(ctx, acct_snapshot.GetAccountAssetSnapshotAtOrBeforeParams{
			AccountID:   *accountID,
			Exchange:    exStr,
			Asset:       k.code,
			WalletType:  k.wt,
			EffectiveTs: start,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "baseline snap %s/%s: %v\n", k.wt, k.code, err)
			os.Exit(1)
		}
		baseTotal := decimal.Zero
		if baseRow != nil {
			baseTotal = utils.Decimal.PgNumericToDecimal(baseRow.Total)
		}
		exp := baseTotal.Add(d)

		endRow, err := db.AcctSnapshotRepo.GetAccountAssetSnapshotAtOrBefore(ctx, acct_snapshot.GetAccountAssetSnapshotAtOrBeforeParams{
			AccountID:   *accountID,
			Exchange:    exStr,
			Asset:       k.code,
			WalletType:  k.wt,
			EffectiveTs: end,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "end snap %s/%s: %v\n", k.wt, k.code, err)
			os.Exit(1)
		}
		row := assetCompareRow{
			Asset:     k.code,
			Wallet:    string(k.wt),
			BaseTotal: baseTotal.String(),
			Delta:     d.String(),
			Expected:  exp.String(),
			SnapRef:   "start/end 时点快照（<= end 最近）",
		}
		if endRow == nil {
			row.Actual = "—"
			row.Diff = "—"
			row.Status = "MISSING"
			exit = 1
			fmt.Printf("asset=%s wallet=%s MISSING_SNAPSHOT_AT_END\n", k.code, k.wt)
		} else {
			act := utils.Decimal.PgNumericToDecimal(endRow.Total)
			diff := act.Sub(exp)
			ok := diff.Abs().LessThanOrEqual(tol)
			st := "OK"
			if !ok {
				st = "MISMATCH"
				exit = 1
			}
			row.Actual = act.String()
			row.Diff = diff.String()
			row.Status = st
			row.SnapRef = fmt.Sprintf("end 快照 effective_ts=%s id=%d", endRow.EffectiveTs.Format(time.RFC3339Nano), endRow.ID)
			fmt.Printf("\nasset=%s wallet=%s base_total@start=%s sum_delta_total=%s expected_total@end=%s actual_total@end=%s diff_total=%s %s\n",
				k.code, k.wt, baseTotal.String(), d.String(), exp.String(), act.String(), diff.String(), st)
		}
		rep.AssetRows = append(rep.AssetRows, row)
	}

	htmlPath := strings.TrimSpace(*htmlOut)
	if htmlPath == "" {
		var err error
		htmlPath, err = defaultReportPath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "report path: %v\n", err)
			os.Exit(1)
		}
	}
	rep.HTMLPath = htmlPath
	rep.ExitCode = exit
	if err := writeHTMLReport(htmlPath, rep); err != nil {
		fmt.Fprintf(os.Stderr, "write html: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\nHTML 报告: %s\n", htmlPath)
	if !*noBrowser {
		if err := openInBrowser(htmlPath); err != nil {
			fmt.Fprintf(os.Stderr, "open browser: %v\n", err)
		}
	}
	os.Exit(exit)
}

func sortedAssetKeys(m map[assetKey]decimal.Decimal) []assetKey {
	keys := make([]assetKey, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].wt != keys[j].wt {
			return keys[i].wt < keys[j].wt
		}
		return keys[i].code < keys[j].code
	})
	return keys
}

func strPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func validateSpotOrderForAudit(ord *orders.Order) error {
	if ord == nil {
		return fmt.Errorf("nil order")
	}
	st := ctypes.OrderStatus(strings.ToUpper(strings.TrimSpace(ord.Status)))
	if st != ctypes.OrderStatusDone && st != ctypes.OrderStatusPartialDone {
		return fmt.Errorf("status %s not DONE/PARTIAL_DONE", ord.Status)
	}
	sym, err := ctypes.ParseSymbol(ord.Symbol)
	if err != nil {
		return err
	}
	if sym.Type != ctypes.MarketTypeSpot {
		return fmt.Errorf("not spot symbol %s", ord.Symbol)
	}
	execQty := utils.Decimal.PgNumericToDecimal(ord.ExecutedQty)
	if !execQty.GreaterThan(decimal.Zero) {
		return fmt.Errorf("executed_qty not positive")
	}
	return nil
}

// spotOrderTotalDelta 与 virtual_sub 现货成交一次入账语义一致：按当前行累计量一次性计入（非多笔部分成交分段 floor）。
func spotOrderTotalDelta(ord *orders.Order, ex ctypes.Exchange) (map[assetKey]decimal.Decimal, error) {
	sym, err := ctypes.ParseSymbol(ord.Symbol)
	if err != nil {
		return nil, err
	}
	wt := walletTypeForSpot(ex)
	base := strings.ToUpper(sym.Base)
	quote := strings.ToUpper(sym.Quote)

	execQty := utils.Decimal.PgNumericToDecimal(ord.ExecutedQty)
	execQuote := utils.Decimal.PgNumericToDecimal(ord.ExecutedPrice)
	avgPrice := utils.Decimal.PgNumericToDecimal(ord.AvgPrice)
	if execQuote.LessThanOrEqual(decimal.Zero) && avgPrice.GreaterThan(decimal.Zero) {
		execQuote = execQty.Mul(avgPrice)
	}

	out := make(map[assetKey]decimal.Decimal)
	if ord.IsBuy {
		add(out, wt, quote, execQuote.Neg())
		add(out, wt, base, execQty)
	} else {
		add(out, wt, base, execQty.Neg())
		add(out, wt, quote, execQuote)
	}

	if ord.Fee.Valid {
		fee := utils.Decimal.PgNumericToDecimal(ord.Fee)
		if fee.IsNegative() {
			fee = fee.Neg()
		}
		if fee.GreaterThan(decimal.Zero) {
			fa := ""
			if ord.FeeAsset != nil {
				fa = strings.ToUpper(strings.TrimSpace(*ord.FeeAsset))
			}
			if fa == "" {
				return nil, fmt.Errorf("fee > 0 but fee_asset empty")
			}
			add(out, wt, fa, fee.Neg())
		}
	}
	return out, nil
}

func add(m map[assetKey]decimal.Decimal, wt acct_snapshot.WalletType, code string, d decimal.Decimal) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" || d.IsZero() {
		return
	}
	k := assetKey{wt: wt, code: code}
	m[k] = m[k].Add(d)
}

func walletTypeForSpot(ex ctypes.Exchange) acct_snapshot.WalletType {
	wt := ctypes.GetWalletType(ex, ctypes.MarketTypeSpot)
	return acct_snapshot.WalletType(string(wt))
}

func listSpotOrdersInUpdatedRange(ctx context.Context, db *repos.Entity, accountID, exchange, symbol string, start, end time.Time) ([]orders.Order, error) {
	const q = `
SELECT id, bot_id, account_id, order_id, client_order_id, drived_order_id,
       order_type, algo_type, source, exchange, symbol, side, is_buy,
       price, quantity, executed_qty, executed_price, avg_price,
       reduce_only, post_only, tif, conditions, detail, status, reject_reason,
       created_ts, working_ts, finished_ts, updated_ts, locked, locked_asset,
       fee, fee_asset, realized_pnl, pnl_asset, fanout, created_at, updated_at
FROM orders
WHERE account_id = $1
  AND exchange = $2
  AND symbol = $3
  AND updated_ts >= $4
  AND updated_ts <= $5
  AND UPPER(status) IN ('DONE', 'PARTIAL_DONE')
  AND executed_qty > 0
ORDER BY updated_ts ASC, id ASC`
	rows, err := db.ConnPool.WConn().WQuery(ctx, "audit.list_orders_updated_range", q, accountID, exchange, symbol, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []orders.Order
	for rows.Next() {
		var o orders.Order
		if err := rows.Scan(
			&o.ID, &o.BotID, &o.AccountID, &o.OrderID, &o.ClientOrderID, &o.DrivedOrderID,
			&o.OrderType, &o.AlgoType, &o.Source, &o.Exchange, &o.Symbol, &o.Side, &o.IsBuy,
			&o.Price, &o.Quantity, &o.ExecutedQty, &o.ExecutedPrice, &o.AvgPrice,
			&o.ReduceOnly, &o.PostOnly, &o.Tif, &o.Conditions, &o.Detail, &o.Status, &o.RejectReason,
			&o.CreatedTs, &o.WorkingTs, &o.FinishedTs, &o.UpdatedTs, &o.Locked, &o.LockedAsset,
			&o.Fee, &o.FeeAsset, &o.RealizedPnl, &o.PnlAsset, &o.Fanout, &o.CreatedAt, &o.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
