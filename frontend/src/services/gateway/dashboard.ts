/**
 * Dashboard 数据分析与监控 API
 */
import { request } from '@umijs/max';
import { AccountStatus, queryAccounts } from './account';
import type { Bot } from './strategy';
import { BotStatus, queryBotBalance, queryBots } from './strategy';

export type DashboardOverview = {
  strategyTotal: number;
  botRunning: number;
  botTotal: number;
  accountOnline: number;
  accountTotal: number;
};

const QUERY_DASHBOARD_OVERVIEW = `
  query DashboardOverview {
    Result: DashboardOverview {
      strategyTotal
      botRunning
      botTotal
      accountOnline
      accountTotal
    }
  }
`;

export type BotProfitRankItem = {
  bot: Bot;
  notional: string;
  unRealizedProfit: string;
  notional24HChange: string;
};

export type AccountProfitRankItem = {
  id: string;
  name: string;
  exchange: string;
  status: string;
  notional24HChange: string;
  notional: string;
  unRealizedProfit: string;
};

/**
 * 获取 Dashboard 概览数据（策略库、Bot、账户）
 * 使用后端 SQL COUNT 接口，避免全量拉取
 */
export async function fetchDashboardOverview() {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_DASHBOARD_OVERVIEW,
    }),
  });
  return response.data.Result ?? { strategyTotal: 0, botRunning: 0, botTotal: 0, accountOnline: 0, accountTotal: 0 };
}

/**
 * 获取 Bot 收益排行（按 24h 变化倒排，取前 N）
 */
export async function fetchBotProfitRank(limit: number = 5): Promise<BotProfitRankItem[]> {
  const result = await queryBots({ pageSize: 100, current: 1, status: BotStatus.Running });
  const bots = result?.list ?? [];
  if (bots.length === 0) return [];

  const items: BotProfitRankItem[] = [];
  const balances = await Promise.all(
    bots.map(async (bot: Bot) => {
      try {
        const balance = await queryBotBalance(bot.id);
        return { bot, balance };
      } catch {
        return { bot, balance: null };
      }
    }),
  );

  for (const { bot, balance } of balances) {
    if (balance) {
      items.push({
        bot,
        notional: balance.notional ?? '0',
        unRealizedProfit: balance.unRealizedProfit ?? '0',
        notional24HChange: balance.notional24HChange ?? '0',
      });
    }
  }

  // 按 notional24HChange 数值降序
  items.sort((a, b) => {
    const va = parseFloat(a.notional24HChange);
    const vb = parseFloat(b.notional24HChange);
    return vb - va;
  });

  return items.slice(0, limit);
}

/**
 * 获取账户 24h 收益排行（仅在线账户，按 24h 变化倒排取前 N）
 */
export async function fetchAccountProfitRank(limit: number = 5): Promise<AccountProfitRankItem[]> {
  const result = await queryAccounts({ pageSize: 50, current: 1, status: AccountStatus.Online });
  const list = result?.list ?? [];

  const items: AccountProfitRankItem[] = list
    .filter((a: { stats?: { notional24HChange?: string } }) => a.stats?.notional24HChange != null)
    .map((a: { id: string; name: string; exchange: string; status: string; stats?: { notional?: string; unRealizedProfit?: string; notional24HChange?: string } }) => ({
      id: a.id,
      name: a.name,
      exchange: a.exchange,
      status: a.status,
      notional24HChange: a.stats?.notional24HChange ?? '0',
      notional: a.stats?.notional ?? '0',
      unRealizedProfit: a.stats?.unRealizedProfit ?? '0',
    }));

  items.sort((a, b) => {
    const va = parseFloat(a.notional24HChange);
    const vb = parseFloat(b.notional24HChange);
    return vb - va;
  });

  return items.slice(0, limit);
}

// ========== Monitor 监控 ==========

export type LlmSceneStats = {
  sceneKey: string;
  sceneId: string;
  totalCount: number;
  successCount: number;
  failCount: number;
  successRate: number;
  avgDurationMs: number;
};

export type LlmCompletionStats = {
  totalCount: number;
  successCount: number;
  failCount: number;
  successRate: number;
  sceneStats: LlmSceneStats[];
};

const QUERY_LLM_COMPLETION_STATS = `
  query LlmCompletionStats($startTs: Int!, $endTs: Int!) {
    Result: LlmCompletionStats(startTs: $startTs, endTs: $endTs) {
      totalCount
      successCount
      failCount
      successRate
      sceneStats {
        sceneKey
        sceneId
        totalCount
        successCount
        failCount
        successRate
        avgDurationMs
      }
    }
  }
`;

/**
 * 获取 LLM 调用统计（指定小时数窗口）
 */
export async function fetchLlmCompletionStats(windowHours: number = 1): Promise<LlmCompletionStats | null> {
  const endTs = Math.floor(Date.now() / 1000);
  const startTs = endTs - windowHours * 3600;
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_LLM_COMPLETION_STATS,
      variables: { startTs, endTs },
    }),
  });
  return response?.data?.Result ?? null;
}

// ========== 市场资讯监控 ==========

export type DocumentStatsSummary = {
  totalCount: number;
  successCount: number;
  successRate: number;
  avgPublishToIngestSec: number;
  avgIngestToSuccessSec: number;
};

export type ChannelDocumentCount = {
  source: string;
  provider: string;
  documentCount: number;
  successCount: number;
};

export type DocumentStats = {
  stats: DocumentStatsSummary;
  channelCounts: ChannelDocumentCount[];
};

const QUERY_DOCUMENT_STATS = `
  query DocumentStats($startTs: Int!, $endTs: Int!) {
    Result: DocumentStats(startTs: $startTs, endTs: $endTs) {
      stats {
        totalCount
        successCount
        successRate
        avgPublishToIngestSec
        avgIngestToSuccessSec
      }
      channelCounts {
        source
        provider
        documentCount
        successCount
      }
    }
  }
`;

export async function fetchDocumentStats(windowHours: number = 1): Promise<DocumentStats | null> {
  const endTs = Math.floor(Date.now() / 1000);
  const startTs = endTs - windowHours * 3600;
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_DOCUMENT_STATS,
      variables: { startTs, endTs },
    }),
  });
  return response?.data?.Result ?? null;
}

// ========== Bot Signal 监控 ==========

export type BotSignalTypeStats = {
  botId: number;
  stream: string;
  eventCount: number;
  avgLatencyMs: number;
  maxLatencyMs: number;
};

export type BotSignalStats = {
  stats: BotSignalTypeStats[];
};

const QUERY_BOT_SIGNAL_STATS = `
  query BotSignalStats($startTs: Int!, $endTs: Int!, $botId: Int) {
    Result: BotSignalStats(startTs: $startTs, endTs: $endTs, botId: $botId) {
      stats {
        botId
        stream
        eventCount
        avgLatencyMs
        maxLatencyMs
      }
    }
  }
`;

export async function fetchBotSignalStats(
  windowHours: number = 1,
  botId?: number,
): Promise<BotSignalStats | null> {
  const endTs = Math.floor(Date.now() / 1000);
  const startTs = endTs - windowHours * 3600;
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_BOT_SIGNAL_STATS,
      variables: { startTs, endTs, botId: botId ?? null },
    }),
  });
  return response?.data?.Result ?? null;
}

// ========== 数据流 Connector 监控 ==========

export type StreamStatsItem = {
  exchange: string;
  stream: string;
  eventCount: number;
  avgLatencyMs: number;
  maxLatencyMs: number;
  reconnectCount: number;
};

const QUERY_STREAM_STATS = `
  query StreamStats($windowHours: Int) {
    Result: StreamStats(windowHours: $windowHours) {
      exchange
      stream
      eventCount
      avgLatencyMs
      maxLatencyMs
      reconnectCount
    }
  }
`;

export async function fetchStreamStats(
  windowHours: number = 1,
): Promise<StreamStatsItem[] | null> {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_STREAM_STATS,
      variables: { windowHours },
    }),
  });
 return response?.data?.Result ?? [];
}
