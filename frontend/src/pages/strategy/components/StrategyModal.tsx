import { Modal, Tabs } from 'antd';
import React from 'react';
import { SignalDefinition, Strategy, StrategyParam } from '@/services/gateway/strategy';
import BacktestForm from './BacktestForm';
import StrategyForm from './StrategyForm';

type StrategyModalProps = {
  mode: 'new' | 'edit' | 'readonly' | 'backtest';
  open: boolean;
  onOpenChange: (open: boolean) => void;
  value?: Strategy;
  onFinish?: (formData: {
    name?: string;
    description?: string;
    code?: string;
    params?: StrategyParam[];
    signals?: SignalDefinition[];
  }) => Promise<boolean | void> | void;
};

const StrategyModal: React.FC<StrategyModalProps> = (props) => {
  const { mode, open, onOpenChange, value, onFinish } = props;
  const isReadonly = mode === 'readonly' || mode === 'backtest';

  return (
    <Modal
      title={`${mode === 'new' ? '新建' : mode === 'edit' ? '编辑' : '查看'}策略`}
      width={isReadonly ? '1000px' : '800px'}
      styles={{ body: { paddingTop: 8, paddingBottom: 0, paddingLeft: 8, paddingRight: 8 } }}
      destroyOnHidden={true}
      footer={null}
      open={open}
      onCancel={() => onOpenChange(false)}
      onOk={async () => {
        // 返回 false 阻止 Modal 自动关闭
        // StrategyForm 的 onFinish 会处理完整数据，如果成功会调用 onOpenChange(false) 关闭 Modal
        return false;
      }}
    >
      {mode === 'readonly' || mode === 'backtest' ? (
        <Tabs
          defaultActiveKey={mode === 'backtest' ? 'backtest' : 'edit'}
          items={[
            {
              key: 'edit',
              label: '策略信息',
              children: (
                <StrategyForm
                  mode={mode === 'backtest' ? 'readonly' : mode}
                  value={value}
                  hideSubmitter={false}
                  onFinish={async (formData) => {
                    if (onFinish) {
                      const success = await onFinish(formData);
                      // 如果成功，关闭 Modal
                      if (success !== false) {
                        onOpenChange(false);
                      }
                      return success !== false;
                    }
                    // 如果没有 onFinish，直接关闭 Modal
                    onOpenChange(false);
                    return true;
                  }}
                />
              ),
            },
            {
              key: 'backtest',
              label: '模拟回测',
              children: value ? (
                <BacktestForm strategy={value} runType={1} />
              ) : (
                <div style={{ textAlign: 'center', padding: '40px 0', color: '#999' }}>
                  策略数据不存在
                </div>
              ),
            },
          ]}
        />
      ) : (
        <StrategyForm
          mode={mode}
          value={value}
          hideSubmitter={false}
          onFinish={async (formData) => {
            if (onFinish) {
              const success = await onFinish(formData);
              // 如果成功，关闭 Modal
              if (success !== false) {
                onOpenChange(false);
              }
              return success !== false;
            }
            // 如果没有 onFinish，直接关闭 Modal
            onOpenChange(false);
            return true;
          }}
        />
      )}
    </Modal>
  );
};

export default StrategyModal;
