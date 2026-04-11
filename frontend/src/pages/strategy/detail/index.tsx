import {
  createStrategy,
  queryStrategy,
  SignalDefinition,
  Strategy,
  StrategyParam,
  StrategyStatus,
  updateStrategy,
} from '@/services/gateway/strategy';
import { history, useParams, useSearchParams } from '@umijs/max';
import { Card, message, Tabs } from 'antd';
import { useEffect, useState } from 'react';
import BacktestForm from '../components/BacktestForm';
import StrategyForm from '../components/StrategyForm';

const StrategyDetailPage: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const [searchParams] = useSearchParams();
  const [strategy, setStrategy] = useState<Strategy | null>(null);
  const [loading, setLoading] = useState(true);
  const [activeKey, setActiveKey] = useState<string>('info');

  const isNew = !id || id === 'new';
  const isReadonly = searchParams.has('readonly');
  const mode = isReadonly ? 'readonly' : isNew ? 'new' : 'edit';

  useEffect(() => {
    const tab = searchParams.get('tab');
    if (tab === 'backtest') {
      setActiveKey('backtest');
    }
  }, [searchParams]);

  useEffect(() => {
    const loadStrategy = async () => {
      if (isNew) {
        setLoading(false);
        setStrategy({
          id: '',
          name: '',
          description: '',
          version: '1',
          params: [],
          signals: [],
          code: '',
          status: StrategyStatus.Draft,
          createdAt: 0,
          updatedAt: 0,
        });
        return;
      }
      if (!id) return;
      try {
        const res = await queryStrategy(id);
        setStrategy(res);
      } catch (error) {
        console.error('加载策略失败:', error);
        message.error('加载策略失败，请稍后重试');
      } finally {
        setLoading(false);
      }
    };
    loadStrategy();
  }, [id, isNew]);

  const handleSave = async (formData: {
    name?: string;
    description?: string;
    code?: string;
    params?: StrategyParam[];
    signals?: SignalDefinition[];
  }) => {
    try {
      if (isNew) {
        const res = await createStrategy({
          name: formData.name || '',
          description: formData.description || '',
          code: formData.code || '',
          params: formData.params || [],
          signals: formData.signals || [],
        });
        if (!res.errors && res.data?.Result) {
          const created = res.data.Result;
          message.success('创建成功');
          setStrategy(created);
          history.replace(`/strategy/${created.id}`);
          return true;
        }
      } else if (strategy) {
        const res = await updateStrategy({
          id: strategy.id,
          version: strategy.version,
          name: formData.name || '',
          description: formData.description || '',
          code: formData.code || '',
          params: formData.params || [],
          signals: formData.signals || [],
        });
        if (!res.errors && res.data?.Result) {
          const updated = res.data.Result;
          message.success('保存成功');
          setStrategy(updated);
          history.replace(`/strategy/${updated.id}`);
          return true;
        }
      }
    } catch (error) {
      console.error('保存失败:', error);
      message.error('保存失败，请稍后重试');
    }
    return false;
  };

  return (
    <Card title={'策略详情'} styles={{ body: { paddingTop: 6 } }} loading={loading}>
      <Tabs
        activeKey={activeKey}
        onChange={setActiveKey}
        items={[
          {
            key: 'info',
            label: '策略信息',
            children: (
              <StrategyForm
                mode={mode}
                value={strategy || undefined}
                onFinish={handleSave}
                onStrategyChange={(s) => {
                  setStrategy((prev) => (prev ? { ...prev, ...s } : null));
                }}
              />
            ),
          },
          {
            key: 'backtest',
            label: '模拟回测',
            children: (
              <BacktestForm strategy={strategy || undefined} runType={1} />
            )
          },
        ]}
      />
    </Card>
  );
};

export default StrategyDetailPage;
