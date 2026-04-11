package extractor

import (
	"testing"

	"github.com/rs/zerolog/log"
)

func Test_extractor(t *testing.T) {
	rules := []Rule{
		{
			Type:    RuleTypeXPath,
			Pattern: `//text()`,
			Group:   1,
		},
		{
			Type:    RuleTypeXPath,
			Pattern: `//a/@href`,
			Group:   1,
		},
	}

	for _, rule := range rules {
		result, err := Extract("销毁，是Uniswap最后的底牌<a href=\"https://m.theblockbeats.info/news/60156?from=telegram\"> - 链接</a>", "html", rule)
		if err != nil {
			t.Fatal(err)
		}
		log.Info().Interface("result", result).Msg("result")
	}
}
