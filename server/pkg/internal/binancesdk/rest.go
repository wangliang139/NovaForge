package binancesdk

import (
	"context"
	"errors"

	binance "github.com/adshao/go-binance/v2"
)

func (c *Client) GetOrderBookDepth(ctx context.Context, symbol string, limit int) (*binance.DepthResponse, error) {
	if limit == 0 {
		return nil, errors.New("limit is required")
	}
	response, err := c.client.NewDepthService().Symbol(symbol).Limit(limit).Do(ctx)
	if err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) GetExchangeInfo(ctx context.Context, symbols, permissions []string) (*binance.ExchangeInfo, error) {
	response, err := c.client.NewExchangeInfoService().Symbols(symbols...).Permissions(permissions...).Do(ctx)
	if err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) GetTickerPrice(ctx context.Context, symbol string) (*string, error) {
	if len(symbol) == 0 {
		return nil, errors.New("symbol is required")
	}
	response, err := c.client.NewListPricesService().Symbol(symbol).Do(ctx)
	if err != nil {
		return nil, err
	}
	if len(response) == 0 {
		return nil, nil
	}
	return &response[0].Price, nil
}

func (c *Client) GetSymbolKlines(ctx context.Context, symbol, interval string, startTime, endTime *int64, limit *int) ([]*binance.Kline, error) {
	if len(symbol) == 0 {
		return nil, errors.New("symbol is required")
	}
	request := c.client.NewKlinesService().Symbol(symbol).Interval(interval)
	if startTime != nil {
		request.StartTime(*startTime)
	}
	if endTime != nil {
		request.EndTime(*endTime)
	}
	if limit != nil {
		request.Limit(*limit)
	}
	response, err := request.Do(ctx)
	if err != nil {
		return nil, err
	}
	return response, err
}
