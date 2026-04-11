package document

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/kelseyhightower/envconfig"
	"github.com/rs/zerolog/log"
	"github.com/stumble/wpgx"
	"github.com/wangliang139/llt-trade/server/pkg/internal/extractor"
	"github.com/wangliang139/llt-trade/server/pkg/repos"
	"github.com/wangliang139/llt-trade/server/pkg/types"
)

func Test_parseTimeFormat(t *testing.T) {
	layout, result, err := parseTimeFormat("11-11 19:13", "{$YYYY:2024}{-}01-02 15:04{:}{$ss:00}")
	if err != nil {
		t.Errorf("failed to parse time format: %v", err)
		return
	}
	t.Logf("layout: %s, result: %s", layout, result)

	tim, err := time.Parse(layout, result)
	if err != nil {
		t.Errorf("failed to parse time: %v", err)
		return
	}
	t.Logf("time: %s", tim.Format(time.RFC3339))
}

func Test_parseTime(t *testing.T) {
	tm := "2025-11-11 19:13"
	tim, err := time.Parse("2006-01-02 15:04", tm)
	if err != nil {
		t.Errorf("failed to parse time: %v", err)
		return
	}
	log.Info().Time("time", tim).Msg("time")
}

func Test_extract(t *testing.T) {
	message := `<b>UNI 24小时涨超10%，市值升至71.92亿美元</b>

BlockBeats 消息，11 月 11 日，据 HTX 行情数据，UNI 24 小时涨幅扩大至 10.91%，市值升至 71.92 亿美元。

此前 <a href="https://www.theblockbeats.info/flash/319915">Uniswap 团队发起提案，拟开启协议手续费开关</a>。

原文链接 <a href="https://m.theblockbeats.info/flash/319916?from=telegram">https://m.theblockbeats.info/flash/319916</a>`

	// 根据 message 格式，自动推断 title/content/url 的提取正则
	// title: <b> ... </b>，在开头
	// content: title 和 —— <a 之间
	// url: <a href="...">，最后一处链接
	extractCfg := types.ExtractCfg{
		Plans: []types.ExtractPlan{
			{
				SeqNo:      1,
				MatchRegex: `^<b>(.*?)</b>\s+([\s\S]+)原文链接 <a href="(.*?)">.*?</a>$`,
				Fields: []types.ExtractField{
					{
						Key: "title",
						Rule: extractor.Rule{
							Type:    extractor.RuleTypeRegex,
							Pattern: `^<b>(.*?)</b>\s+([\s\S]+)原文链接 <a href="(.*?)">.*?</a>$`,
							Group:   1,
						},
					},
					{
						Key: "content",
						Rule: extractor.Rule{
							Type:    extractor.RuleTypeRegex,
							Pattern: `^<b>(.*?)</b>\s+([\s\S]+)原文链接 <a href="(.*?)">.*?</a>$`,
							Group:   2,
						},
					},
					{
						Key: "url",
						Rule: extractor.Rule{
							Type:    extractor.RuleTypeRegex,
							Pattern: `^<b>(.*?)</b>\s+([\s\S]+)原文链接 <a href="(.*?)">.*?</a>$`,
							Group:   3,
						},
					},
				},
			},
		},
	}

	str, err := sonic.MarshalString(extractCfg)
	if err != nil {
		log.Error().Err(err).Msg("failed to marshal extract cfg")
	}
	// log.Info().Str("plans", str).Msg("plans")
	t.Logf("extract cfg: %s", str)

	result, err := extract(context.Background(), extractCfg, message)
	if err != nil {
		t.Errorf("failed to extract: %v", err)
		return
	}
	log.Info().Interface("result", result).Msg("result")
}

func Test_extractByDbConfig(t *testing.T) {
	os.Setenv("POSTGRES_PASSWORD", "postgres")
	os.Setenv("POSTGRES_APPNAME", "llt-data")
	os.Setenv("POSTGRES_DBNAME", "llt_data_db")
	os.Setenv("POSTGRES_PASSWORD", "my-secret")
	var wpgxConfig wpgx.Config
	envconfig.MustProcess("postgres", &wpgxConfig)
	log.Info().Msgf("wpgx config: %+v", &wpgxConfig)

	pool, err := wpgx.NewPool(context.Background(), &wpgxConfig)
	if err != nil {
		t.Fatalf("failed to create wpgx pool: %v", err)
	}
	defer pool.Close()

	db := repos.New(pool, nil)

	channelId := int64(1320910809)
	message := `TD COWEN将路威酩轩目标价从550欧元上调至685欧元。

(2025-11-12 14:02)`

	channelCfg, err := db.TgChannelRepo.GetById(context.Background(), channelId)
	if err != nil {
		t.Errorf("failed to get channel: %v", err)
		return
	}

	if channelCfg == nil {
		t.Errorf("channel config not found")
		return
	}

	t.Logf("channel extract cfg: %s", string(channelCfg.ExtractCfg))

	var extractCfg types.ExtractCfg
	if err := sonic.Unmarshal(channelCfg.ExtractCfg, &extractCfg); err != nil {
		t.Errorf("failed to unmarshal structure: %v", err)
		return
	}

	extractResult, err := extract(context.Background(), extractCfg, message)
	if err != nil {
		t.Errorf("failed to extract: %v", err)
		return
	}
	log.Info().Interface("extractResult", extractResult).Msg("extractResult")
}
