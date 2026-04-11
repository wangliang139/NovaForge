package binancesdk

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	binance "github.com/adshao/go-binance/v2"
	"github.com/rs/zerolog/log"
)

func Test(t *testing.T) {
	ApiKey := os.Getenv("BINANCE_API_KEY")
	ApiSecret := os.Getenv("BINANCE_API_SECRET")

	config := Config{
		ApiKey:    ApiKey,
		ApiSecret: ApiSecret,
		IsDebug:   true,
	}

	ctx := context.Background()

	sdk := NewClient(config)
	err := sdk.client.NewPingService().Do(ctx)
	if err != nil {
		t.Fatal(err)
	}
	serverTime, err := sdk.client.NewServerTimeService().Do(ctx)
	if err != nil {
		t.Fatal(err)
	}
	log.Info().Interface("serverTimeResponse", serverTime).Send()
	log.Info().Int64("gap", time.Now().UnixMilli()-int64(serverTime)).Send()

	symbol := "GUSDT"
	exchangeInfoResponse, err := sdk.client.NewExchangeInfoService().Symbols(symbol).Do(ctx)
	if err != nil {
		t.Fatal(err)
	}
	log.Info().Interface("exchangeInfoResponse", exchangeInfoResponse).Send()

	orderBookResponse, err := sdk.client.NewDepthService().Symbol(symbol).Limit(5).Do(ctx)
	if err != nil {
		t.Fatal(err)
	}
	log.Info().Interface("orderBookResponse", orderBookResponse).Send()

	accountResponse, err := sdk.client.NewGetAccountService().Do(ctx)
	if err != nil {
		t.Fatal(err)
	}
	log.Info().Interface("accountResponse", accountResponse).Send()
}

func Test_Stream(t *testing.T) {
	ApiKey := os.Getenv("BINANCE_API_KEY")
	ApiSecret := os.Getenv("BINANCE_API_SECRET")

	config := Config{
		ApiKey:    ApiKey,
		ApiSecret: ApiSecret,
		IsDebug:   true,
	}

	symbol := "GUSDT"

	wsDepthHandler := func(event *binance.WsDepthEvent) {
		// fmt.Println(binance_connector.PrettyPrint(event))
	}

	errHandler := func(err error) {
		fmt.Println(err)
	}

	ctx := context.Background()

	client := NewClient(config)
	doneCh, stopCh, err := client.ListenOrderBookEvent(ctx, []string{symbol}, errHandler, wsDepthHandler)
	if err != nil {
		fmt.Println(err)
		return
	}

	go func() {
		time.Sleep(30 * time.Second)
		stopCh <- struct{}{} // use stopCh to stop streaming
	}()

	<-doneCh
}

func Test_Sapi(t *testing.T) {
	ApiKey := os.Getenv("BINANCE_API_KEY")
	ApiSecret := os.Getenv("BINANCE_API_SECRET")

	config := Config{
		ApiKey:    ApiKey,
		ApiSecret: ApiSecret,
		IsDebug:   true,
	}

	ctx := context.Background()
	ch := make(chan struct{})

	errHandler := func(err error) {
		log.Error().Err(err).Send()
	}

	wsSapiHandler := func(event *WsSapiEvent) {
		log.Info().Interface("event", event).Send()
		ch <- struct{}{}
	}

	client := NewClient(config)
	doneCh, stopCh, err := client.ListenSapiWsEvent(ctx, "com_announcement_en", errHandler, wsSapiHandler)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}

	go func() {
		<-ch
		stopCh <- struct{}{} // use stopCh to stop streaming
	}()

	<-doneCh
}
