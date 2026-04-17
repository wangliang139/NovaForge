package main

import (
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// htmlReport 稽核结果（用于终端 + HTML）。
type htmlReport struct {
	GeneratedAt   time.Time
	Mode          string
	AccountID     string
	Tolerance     string
	MetaLines     []string
	AssetRows     []assetCompareRow
	FanoutRows    []fanoutCompareRow
	FanoutQtyRows []fanoutQtyCompareRow
	QtyTolerance  string
	OrderRows     []orderAuditRow
	Warnings      []string
	ExitCode      int
	HTMLPath      string
}

type assetCompareRow struct {
	Asset     string
	Wallet    string
	BaseTotal string
	Delta     string
	Expected  string
	Actual    string
	Diff      string
	Status    string
	SnapRef   string
}

type orderAuditRow struct {
	OrderID    string
	UpdatedTs  string
	IsBuy      bool
	ExecQty    string
	ExecQuote  string
	Fee        string
	FeeAsset   string
	SkipReason string
}

// fanoutCompareRow 父单 fanout 子账户份额比对行。
type fanoutCompareRow struct {
	SubAccountID  string
	DBShare       string
	ExpectedShare string
	Diff          string
	Status        string
}

// fanoutQtyCompareRow 子账户订单 quantity（对应 OriginalQty）与缩放期望值比对。
type fanoutQtyCompareRow struct {
	SubAccountID        string
	ExpectedOriginalQty string
	DBOriginalQty       string
	Diff                string
	Lookup              string
	Status              string
	Note                string
}

const reportTemplate = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>virtual_sub 现货稽核</title>
<style>
:root { --ok:#1b5e20; --bad:#b71c1c; --muted:#616161; --border:#e0e0e0; }
* { box-sizing: border-box; }
body { font-family: ui-sans-serif, system-ui, "Segoe UI", Roboto, "PingFang SC", "Microsoft YaHei", sans-serif; margin: 0; padding: 1.25rem 1.5rem; color: #212121; background: #fafafa; }
h1 { font-size: 1.35rem; font-weight: 600; margin: 0 0 0.5rem; }
.sub { color: var(--muted); font-size: 0.875rem; margin-bottom: 1.25rem; line-height: 1.5; }
.badge { display: inline-block; padding: 0.2rem 0.5rem; border-radius: 4px; font-size: 0.8rem; font-weight: 600; }
.badge-ok { background: #e8f5e9; color: var(--ok); }
.badge-fail { background: #ffebee; color: var(--bad); }
section { margin-bottom: 1.75rem; }
h2 { font-size: 1rem; font-weight: 600; margin: 0 0 0.75rem; }
table { width: 100%; border-collapse: collapse; background: #fff; border: 1px solid var(--border); border-radius: 6px; overflow: hidden; }
th, td { text-align: left; padding: 0.5rem 0.65rem; font-size: 0.875rem; border-bottom: 1px solid var(--border); }
th { background: #f5f5f5; font-weight: 600; }
tr:last-child td { border-bottom: none; }
tr.row-ok td:first-child { border-left: 3px solid var(--ok); }
tr.row-bad td:first-child { border-left: 3px solid var(--bad); }
tr.row-warn td:first-child { border-left: 3px solid #f57f17; }
.warn-list { background: #fff8e1; border: 1px solid #ffe082; border-radius: 6px; padding: 0.75rem 1rem; font-size: 0.875rem; margin: 0; }
.warn-list li { margin: 0.25rem 0; }
.mono { font-family: ui-monospace, "Cascadia Code", Consolas, monospace; font-size: 0.82rem; }
.footer { margin-top: 2rem; font-size: 0.8rem; color: var(--muted); }
</style>
</head>
<body>
<h1>虚拟子账户订单稽核报告</h1>
<div class="sub">
  <div><strong>生成时间</strong>：{{.GeneratedAt.Format "2006-01-02 15:04:05 MST"}}</div>
  <div><strong>模式</strong>：{{.Mode}} &nbsp;|&nbsp; <strong>账户</strong>：<span class="mono">{{.AccountID}}</span> &nbsp;|&nbsp; <strong>份额容差</strong>：{{.Tolerance}}{{if .QtyTolerance}} &nbsp;|&nbsp; <strong>originalQty 容差</strong>：{{.QtyTolerance}}{{end}}</div>
  <div><strong>结果</strong>：
    {{if eq .ExitCode 0}}<span class="badge badge-ok">全部通过</span>{{else}}<span class="badge badge-fail">存在差异或缺失</span>{{end}}
  </div>
  {{range .MetaLines}}<div>{{.}}</div>{{end}}
</div>

{{if .Warnings}}
<section>
<h2>警告 / 跳过</h2>
<ul class="warn-list">{{range .Warnings}}<li>{{.}}</li>{{end}}</ul>
</section>
{{end}}

{{if .FanoutRows}}
<section>
<h2>多 Bot 父单 fanout 分摊</h2>
<table>
  <thead>
    <tr>
      <th>子账户</th><th>DB fanout 份额</th><th>重算份额</th><th>差额</th><th>状态</th>
    </tr>
  </thead>
  <tbody>
  {{range .FanoutRows}}
    <tr class="{{if eq .Status "OK"}}row-ok{{else if eq .Status "MISSING_IN_DB"}}row-warn{{else}}row-bad{{end}}">
      <td class="mono">{{.SubAccountID}}</td>
      <td class="mono">{{.DBShare}}</td>
      <td class="mono">{{.ExpectedShare}}</td>
      <td class="mono">{{.Diff}}</td>
      <td><strong>{{.Status}}</strong></td>
    </tr>
  {{end}}
  </tbody>
</table>
</section>
{{end}}

{{if .FanoutQtyRows}}
<section>
<h2>子单 originalQty（orders.quantity）比对</h2>
<p class="sub" style="margin-top:-0.5rem">期望来自 BuildFanoutScaledOrdersForAudit，与 multi_bot 缩放一致；未接 market engine 时用默认精度可能与线上略有差异。SKIPPED_BELOW_MIN_STEP 表示过小不下发子流。</p>
<table>
  <thead>
    <tr>
      <th>子账户</th><th>期望 originalQty</th><th>子单 quantity</th><th>差额</th><th>匹配方式</th><th>状态</th><th>备注</th>
    </tr>
  </thead>
  <tbody>
  {{range .FanoutQtyRows}}
    <tr class="{{if or (eq .Status "OK") (eq .Status "SKIPPED_BELOW_MIN_STEP")}}row-ok{{else if eq .Status "WARN_SUB_ROW_UNEXPECTED"}}row-warn{{else}}row-bad{{end}}">
      <td class="mono">{{.SubAccountID}}</td>
      <td class="mono">{{.ExpectedOriginalQty}}</td>
      <td class="mono">{{.DBOriginalQty}}</td>
      <td class="mono">{{.Diff}}</td>
      <td class="mono">{{.Lookup}}</td>
      <td><strong>{{.Status}}</strong></td>
      <td>{{.Note}}</td>
    </tr>
  {{end}}
  </tbody>
</table>
</section>
{{end}}

{{if .AssetRows}}
<section>
<h2>资产比对（仅 total）</h2>
<table>
  <thead>
    <tr>
      <th>资产</th><th>钱包</th><th>基线余额(total)</th><th>理论增量(total)</th><th>预期(total)</th><th>实际(total)</th><th>差额(total)</th><th>状态</th><th>快照参考</th>
    </tr>
  </thead>
  <tbody>
  {{range .AssetRows}}
    <tr class="{{if eq .Status "OK"}}row-ok{{else if eq .Status "MISSING"}}row-warn{{else}}row-bad{{end}}">
      <td class="mono">{{.Asset}}</td>
      <td class="mono">{{.Wallet}}</td>
      <td class="mono">{{.BaseTotal}}</td>
      <td class="mono">{{.Delta}}</td>
      <td class="mono">{{.Expected}}</td>
      <td class="mono">{{.Actual}}</td>
      <td class="mono">{{.Diff}}</td>
      <td><strong>{{.Status}}</strong></td>
      <td>{{.SnapRef}}</td>
    </tr>
  {{end}}
  </tbody>
</table>
</section>
{{end}}

{{if .OrderRows}}
<section>
<h2>区间内订单（range）</h2>
<table>
  <thead>
    <tr>
      <th>订单 ID</th><th>updated_ts</th><th>方向</th><th>成交量</th><th>成交额</th><th>手续费</th><th>备注</th>
    </tr>
  </thead>
  <tbody>
  {{range .OrderRows}}
    <tr>
      <td class="mono">{{.OrderID}}</td>
      <td class="mono">{{.UpdatedTs}}</td>
      <td>{{if .IsBuy}}买{{else}}卖{{end}}</td>
      <td class="mono">{{.ExecQty}}</td>
      <td class="mono">{{.ExecQuote}}</td>
      <td class="mono">{{if .FeeAsset}}{{.Fee}} {{.FeeAsset}}{{else}}{{.Fee}}{{end}}</td>
      <td>{{.SkipReason}}</td>
    </tr>
  {{end}}
  </tbody>
</table>
</section>
{{end}}

<p class="footer">报告文件：<span class="mono">{{.HTMLPath}}</span></p>
</body>
</html>
`

func writeHTMLReport(path string, rep *htmlReport) error {
	tpl, err := template.New("audit").Parse(reportTemplate)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := tpl.Execute(f, rep); err != nil {
		return err
	}
	return f.Sync()
}

func openInBrowser(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", abs).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", abs).Start()
	default:
		return exec.Command("xdg-open", abs).Start()
	}
}

func defaultReportPath() (string, error) {
	dir := os.TempDir()
	name := fmt.Sprintf("novaforge-audit-%s.html", time.Now().Format("20060102-150405"))
	return filepath.Join(dir, name), nil
}
