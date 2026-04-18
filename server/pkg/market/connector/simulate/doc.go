// Package simulate implements paper-trading against live public market data: the SimExchange core
// (L2 depth, portfolio, SimBook, orders) lives alongside the market Connector that multiplexes
// public REST/WebSocket feeds per exchange account.
//
// Connector 实现集中在 connector.go；公开行情订阅与 fan-out 见 stream_hub.go；
// 撮合与持仓等核心逻辑见 sim_exchange.go、portfolio.go、depth.go 等。
package simulate
