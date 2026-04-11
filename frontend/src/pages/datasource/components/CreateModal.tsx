import { Exchange } from '@/global.types';
import { api } from '@/services/gateway';
import { enumToOptions } from '@/utils/dict';
import { getExchangeLogo } from '@/utils/market';
import {
  ModalForm,
  ProForm,
  ProFormDateTimeRangePicker,
  ProFormField,
  ProFormSelect,
  ProFormText,
  ProFormTextArea,
} from '@ant-design/pro-components';
import { Form, Select } from 'antd';
import dayjs from 'dayjs';
import React, { useEffect, useState } from 'react';
import { IsMarketSignal, KlineIntervalOptions, SignalType } from '@/services/gateway/strategy';

type CreateModalProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onFinish?: (formData: {
    name: string;
    description: string;
    type: SignalType;
    exchange?: string;
    symbol?: string;
    props?: string;
    startTs: number;
    endTs: number;
  }) => Promise<boolean | void> | void;
};

const CreateModal: React.FC<CreateModalProps> = (props) => {
  const { open, onOpenChange, onFinish } = props;
  const [form] = Form.useForm();
  const selectedExchange = Form.useWatch('exchange', form);
  const [symbolOptions, setSymbolOptions] = useState<{ label: string; value: string }[]>([]);
  const [loadingSymbols, setLoadingSymbols] = useState(false);
  const type = Form.useWatch('type', form);
  const [loading, setLoading] = useState(false);

  // 根据交易所加载交易对
  useEffect(() => {
    if (!selectedExchange || !open) {
      setSymbolOptions([]);
      // 清空交易对字段
      form.setFieldsValue({ symbol: undefined });
      return;
    }

    setLoadingSymbols(true);
    api
      .queryMarkets({ exchange: selectedExchange })
      .then((markets) => {
        if (markets && markets.length > 0) {
          // 将 markets 转换为选项
          const symbolOptions = markets.map((market: any) => ({
            label: market.symbol,
            value: market.symbol,
          }));
          setSymbolOptions(symbolOptions);
        } else {
          setSymbolOptions([]);
        }
      })
      .catch((error) => {
        console.error('Failed to load symbols:', error);
        setSymbolOptions([]);
      })
      .finally(() => {
        setLoadingSymbols(false);
      });
  }, [selectedExchange, open, form]);

  return (
    <ModalForm
      form={form}
      title="新建数据源"
      width="600px"
      grid={false}
      style={{
        paddingTop: 30,
      }}
      modalProps={{
        confirmLoading: loading,
        destroyOnHidden: true,
      }}
      labelAlign="right"
      layout="vertical"
      open={open}
      loading={loading}
      onOpenChange={onOpenChange}
      onFinish={async (formData) => {
        setLoading(true);
        try {
          if (onFinish) {
            const data = {
              exchange: formData.exchange,
              symbol: formData.symbol,
              startTs: dayjs(formData.dateRange?.[0]).unix(),
              endTs: dayjs(formData.dateRange?.[1]).unix(),
              name: formData.name,
              type: formData.type,
              description: formData?.description || '',
              props: undefined as string | undefined,
            };

            const props: Record<string, any> = {};
            if (formData.type === SignalType.Kline) {
              props['interval'] = formData.interval;
            }

            data.props = JSON.stringify(props);

            let success = await onFinish(data);
            if (!success) {
              return false;
            }
          }
          return true;
        } finally {
          setLoading(false);
        }
      }}
    >
      <ProForm.Group>
        <ProFormText name="name" label="名称" required />
        <ProFormSelect
          name="type"
          label="类型"
          options={enumToOptions(SignalType, 'Unspecified')}
          required
        />
      </ProForm.Group>
      <ProFormTextArea name="description" label="描述" />
      <ProForm.Group>
        <Form.Item
          style={{ width: '120px' }}
          name="exchange"
          label="交易所"
          rules={[
            {
              required: IsMarketSignal(type),
              message: '交易所是必填项',
            },
          ]}
        >
          <Select
            onChange={() => {
              // 当交易所改变时，清空交易对
              form.setFieldsValue({ symbol: undefined });
            }}
          >
            <Select.Option value="binance">
              <img
                alt={Exchange.Binance}
                style={{ display: 'inline', marginLeft: 4 }}
                width={16}
                src={getExchangeLogo(Exchange.Binance)}
              />{' '}
              Binance
            </Select.Option>
            <Select.Option value="binance_test">
              <img
                alt={Exchange.BinanceTest}
                style={{ display: 'inline', marginLeft: 4 }}
                width={16}
                src={getExchangeLogo(Exchange.BinanceTest)}
              />{' '}
              Binance Test
            </Select.Option>
            <Select.Option value="okx">
              <img
                alt={Exchange.OKX}
                style={{ display: 'inline', marginLeft: 4 }}
                width={16}
                src={getExchangeLogo(Exchange.OKX)}
              />{' '}
              OKX
            </Select.Option>
            <Select.Option value="okx_test">
              <img
                alt={Exchange.OKXTest}
                style={{ display: 'inline', marginLeft: 4 }}
                width={16}
                src={getExchangeLogo(Exchange.OKXTest)}
              />{' '}
              OKX Test
            </Select.Option>
          </Select>
        </Form.Item>
        <ProFormField
          name="symbol"
          label="交易对"
          width="md"
          rules={[
            {
              required: IsMarketSignal(type),
              message: '交易对是必填项',
            },
          ]}
        >
          <Select
            allowClear
            showSearch
            style={{ width: '200px' }}
            loading={loadingSymbols}
            options={symbolOptions}
            disabled={!selectedExchange || loadingSymbols}
            placeholder={selectedExchange ? '请选择交易对' : '请先选择交易所'}
            filterOption={(input: string, option?: { label: string; value: string }) => {
              if (!option) return false;
              const searchText = input.toUpperCase();
              const label = option.label?.toUpperCase() || '';
              return label.includes(searchText);
            }}
          />
        </ProFormField>
      </ProForm.Group>

      <ProFormDateTimeRangePicker
        name="dateRange"
        label="时间范围"
        width="lg"
        rules={[
          {
            required: true,
            message: '时间范围是必填项',
          },
        ]}
      />

      {type === SignalType.Kline && (
        <ProFormSelect
          name="interval"
          label="时间间隔"
          width="xs"
          options={KlineIntervalOptions}
          rules={[
            {
              required: true,
              message: '时间间隔是必填项',
            },
          ]}
        />
      )}
    </ModalForm>
  );
};

export default CreateModal;
