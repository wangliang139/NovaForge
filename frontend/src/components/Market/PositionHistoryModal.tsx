import { Exchange } from '@/global.types';
import { api } from '@/services/gateway';
import { type Position } from '@/services/gateway/account';
import type { MarketInfo } from '@/services/gateway/market';
import { queryPositionSnapshotHistory } from '@/services/gateway/market';
import utils from '@/utils';
import { getSideTagInfo } from '@/utils/marketTag';
import { Empty, Modal, Segmented, Spin, Typography, message } from 'antd';
import dayjs from 'dayjs';
import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  Tooltip as RechartsTooltip,
  ResponsiveContainer,
  XAxis,
  YAxis,
} from 'recharts';

type RangeKey = '1d' | '7d' | '30d';

const RANGE_MS: Record<RangeKey, number> = {
  '1d': 24 * 60 * 60 * 1000,
  '7d': 7 * 24 * 60 * 60 * 1000,
  '30d': 30 * 24 * 60 * 60 * 1000,
};

export type PositionSnapshotHistoryModalProps = {
  open: boolean;
  onClose: () => void;
  accountId: string;
  exchange: Exchange;
  position: Pick<Position, 'symbol' | 'side'>;
};

const PositionSnapshotHistoryModal: React.FC<PositionSnapshotHistoryModalProps> = ({
  open,
  onClose,
  accountId,
  exchange,
  position,
}) => {
  const [range, setRange] = useState<RangeKey>('1d');
  const prevOpen = useRef(false);
  const [loading, setLoading] = useState(false);
  const [rows, setRows] = useState<{ tsMs: number; qty: string; entryPrice: string }[]>([]);
  const [pricePrecision, setPricePrecision] = useState(8);

  useEffect(() => {
    if (open && !prevOpen.current) {
      setRange('1d');
    }
    prevOpen.current = open;
  }, [open]);

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    (async () => {
      setLoading(true);
      const endTsMs = Date.now();
      const startTsMs = endTsMs - RANGE_MS[range];
      try {
        const list = await queryPositionSnapshotHistory({
          accountId,
          symbol: position.symbol,
          side: position.side,
          startTsMs,
          endTsMs,
        });
        if (!cancelled) setRows(list);
      } catch (e: any) {
        if (!cancelled) {
          message.error(e?.message || '加载快照历史失败');
          setRows([]);
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [open, range, accountId, position]);

  useEffect(() => {
    if (!open || !exchange || !position.symbol) return;
    let cancelled = false;
    (async () => {
      try {
        const market = (await api.queryMarket({
          exchange,
          symbol: position.symbol,
        })) as MarketInfo | null;
        const pp = market?.pricePrecision;
        if (!cancelled && Number.isInteger(pp) && (pp as number) >= 0) {
          setPricePrecision(pp as number);
        }
      } catch {
        // 获取精度失败时保持默认精度，不打断主流程
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [open, accountId, position.symbol]);

  const title = useMemo(() => {
    const sd = getSideTagInfo(String(position.side || ''));
    return `${position.symbol} · ${sd.text} — 仓位曲线`;
  }, [position]);

  const chartData = useMemo(
    () =>
      rows.map((p) => ({
        label: dayjs(p.tsMs).format('MM-DD HH:mm'),
        tsMs: p.tsMs,
        qty: Number(p.qty),
        entryPrice: Number(p.entryPrice),
        qtyRaw: p.qty,
        entryRaw: p.entryPrice,
      })),
    [rows],
  );

  return (
    <Modal title={title} open={open} onCancel={onClose} footer={null} width={760} destroyOnHidden>
      <div style={{ marginBottom: 12 }}>
        <Segmented<RangeKey>
          value={range}
          onChange={(v) => setRange(v)}
          options={[
            { label: '一天', value: '1d' },
            { label: '一周', value: '7d' },
            { label: '一月', value: '30d' },
          ]}
        />
        <Typography.Text type="secondary" style={{ marginLeft: 12, fontSize: 12 }}>
          相对当前时刻滚动时间窗；数据来自库内快照记录，无记录时为空。
        </Typography.Text>
      </div>
      <Spin spinning={loading}>
        {!loading && rows.length === 0 ? (
          <Empty description="暂无历史" />
        ) : (
          <ResponsiveContainer width="100%" height={340}>
            <LineChart data={chartData}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="label" interval="preserveStartEnd" minTickGap={24} />
              <YAxis yAxisId="qty" domain={['auto', 'auto']} />
              <YAxis yAxisId="price" orientation="right" domain={['auto', 'auto']} />
              <RechartsTooltip<any, any>
                formatter={(value: number, name: string, item: any) => {
                  if (name === '数量') return [utils.math.formatByPrecision(item?.payload?.qtyRaw ?? value, 8), name];
                  if (name === '开仓均价') {
                    return [
                      utils.math.formatByPrecision(item?.payload?.entryRaw ?? value, pricePrecision),
                      name,
                    ];
                  }
                  return [value, name];
                }}
                labelStyle={{ color: '#d89614' }}
                labelFormatter={(_: unknown, payload: any[]) =>
                  payload?.[0]?.payload?.tsMs != null
                    ? dayjs(payload[0].payload.tsMs).format('YYYY-MM-DD HH:mm:ss')
                    : ''
                }
              />
              <Legend />
              <Line yAxisId="qty" type="monotone" dataKey="qty" name="数量" stroke="#1677ff" dot={false} strokeWidth={2} />
              <Line
                yAxisId="price"
                type="monotone"
                dataKey="entryPrice"
                name="开仓均价"
                stroke="#fa8c16"
                dot={false}
                strokeWidth={2}
              />
            </LineChart>
          </ResponsiveContainer>
        )}
      </Spin>
    </Modal>
  );
};

export default PositionSnapshotHistoryModal;
