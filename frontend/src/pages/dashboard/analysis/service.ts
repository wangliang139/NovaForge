import {
  fetchAccountProfitRank,
  fetchBotProfitRank,
  fetchDashboardOverview,
} from '@/services/gateway/dashboard';

export type AnalysisData = {
  overview: Awaited<ReturnType<typeof fetchDashboardOverview>>;
  botProfitRank: Awaited<ReturnType<typeof fetchBotProfitRank>>;
  accountProfitRank: Awaited<ReturnType<typeof fetchAccountProfitRank>>;
  totalAccountNotional: string;
  totalAccount24hChange: string;
};

export async function fetchAnalysisData(): Promise<AnalysisData> {
  const [overview, botProfitRank, accountProfitRank] = await Promise.all([
    fetchDashboardOverview(),
    fetchBotProfitRank(5),
    fetchAccountProfitRank(50), // 多取用于计算总和
  ]);

  const totalNotional = accountProfitRank.reduce((s, a) => s + parseFloat(a.notional || '0'), 0);
  const total24h = accountProfitRank.reduce((s, a) => s + parseFloat(a.notional24HChange || '0'), 0);

  return {
    overview,
    botProfitRank,
    accountProfitRank: accountProfitRank.slice(0, 10),
    totalAccountNotional: totalNotional.toFixed(2),
    totalAccount24hChange: total24h.toFixed(4),
  };
}
