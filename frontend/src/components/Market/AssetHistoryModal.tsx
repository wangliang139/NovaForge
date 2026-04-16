import type { Asset } from '@/services/gateway/account';
import { queryAssetSnapshotHistory } from '@/services/gateway/market';
import utils from '@/utils';
import { getWalletTypeTagInfo } from '@/utils/marketTag';
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

export type AssetSnapshotHistoryModalProps = {
  open: boolean;
  onClose: () => void;
  accountId: string;
  asset: Pick<Asset, 'code' | 'walletType'>;
};

const AssetSnapshotHistoryModal: React.FC<AssetSnapshotHistoryModalProps> = ({
  open,
  onClose,
  accountId,
  asset,
}) => {
  const [range, setRange] = useState<RangeKey>('1d');
  const prevOpen = useRef(false);
  const [loading, setLoading] = useState(false);
  const [rows, setRows] = useState<{ tsMs: number; total: string }[]>([]);

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
        const list = await queryAssetSnapshotHistory({
          accountId,
          walletType: asset.walletType,
          asset: asset.code,
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
  }, [open, range, accountId, asset]);

  const title = useMemo(() => {
    const wt = getWalletTypeTagInfo(String(asset.walletType || ''));
    return `${asset.code} · ${wt.text} — 资产曲线`;
  }, [asset]);

  const chartData = useMemo(
    () =>
      rows.map((p) => ({
        label: dayjs(p.tsMs).format('MM-DD HH:mm'),
        tsMs: p.tsMs,
        total: Number(p.total),
        totalRaw: p.total,
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
              <YAxis domain={['auto', 'auto']} />
              <RechartsTooltip<any, any>
                formatter={(value: number, _name: string, item: any) => [
                  utils.math.formatByPrecision(item?.payload?.totalRaw ?? value, 8),
                  '总额',
                ]}
                labelFormatter={(_: unknown, payload: any[]) =>
                  payload?.[0]?.payload?.tsMs != null
                    ? dayjs(payload[0].payload.tsMs).format('YYYY-MM-DD HH:mm:ss')
                    : ''
                }
              />
              <Legend />
              <Line type="monotone" dataKey="total" name="总额" stroke="#1677ff" dot={false} strokeWidth={2} />
            </LineChart>
          </ResponsiveContainer>
        )}
      </Spin>
    </Modal>
  );
};

export default AssetSnapshotHistoryModal;
