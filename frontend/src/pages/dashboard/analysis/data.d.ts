import { DataItem } from '@antv/g2plot/esm/interface/config';
import { Exchange } from '@/global.types';

export { DataItem };

export interface VisitDataType {
  x: string;
  y: number;
}

export type SearchDataType = {
  index: number;
  keyword: string;
  count: number;
  range: number;
  status: number;
};

export type OfflineDataType = {
  name: string;
  cvr: number;
};

export interface OfflineChartData {
  date: number;
  type: number;
  value: number;
}

export type RadarData = {
  name: string;
  label: string;
  value: number;
};
export type SyncerStat = {
  symbol: string;
  exchange: string;
  type: string;
  status: string;
  credibility: number;
  startAt: number;
  lastSyncTime: number;
  abnormals: {
    time: number;
    msg?: string;
    before: number;
    after: number;
    change: number;
  }[];
};

export interface AnalysisData {
  visitData: DataItem[];
  visitData2: DataItem[];
  salesData: DataItem[];
  searchData: DataItem[];
  offlineData: OfflineDataType[];
  offlineChartData: DataItem[];
  salesTypeData: DataItem[];
  salesTypeDataOnline: DataItem[];
  salesTypeDataOffline: DataItem[];
  radarData: RadarData[];
  syncerStats: SyncerStat[];
}
