import { Exchange } from '@/global.types';
import { api } from '@/services/gateway';
import {
  KlineIntervalOptions,
  SignalDefinition,
  SignalScope,
  SignalScopeOptions,
  SignalType,
  SignalTypeOptions,
} from '@/services/gateway/strategy';
import { getExchangeLogo } from '@/utils/market';
import {
  ModalForm,
  ProForm,
  ProFormDigit,
  ProFormField,
  ProFormSelect,
  ProFormText,
} from '@ant-design/pro-components';
import { Form, Select } from 'antd';
import React, { useEffect, useState } from 'react';

type SignalModalProps = {
  open: boolean;
  value?: SignalDefinition;
  readonly?: boolean;
  onOpenChange: (open: boolean) => void;
  onFinish?: (formData: SignalDefinition) => Promise<boolean | void> | void;
};

// 将 props 数组转换为表单字段值
const propsToFormValues = (props?: string): Record<string, any> => {
  if (!props) return {};
  return JSON.parse(props) as Record<string, any>;
};

// 将表单字段值转换为 props JSON 字符串
const formValuesToProps = (props: Record<string, any> | undefined): string | undefined => {
  if (!props) return undefined;
  return JSON.stringify(props);
};

const SignalModal: React.FC<SignalModalProps> = ({
  open,
  value,
  readonly,
  onOpenChange,
  onFinish,
}) => {
  const [form] = Form.useForm();
  const signalType = Form.useWatch('type', form);
  const scope = Form.useWatch('scope', form);
  const selectedExchange = Form.useWatch('exchange', form);
  const [symbolOptions, setSymbolOptions] = useState<{ label: string; value: string }[]>([]);
  const [loadingSymbols, setLoadingSymbols] = useState(false);

  // 当作用域改变时，如果不是 Target，清空交易所和交易对
  useEffect(() => {
    if (scope && scope !== SignalScope.Target) {
      form.setFieldsValue({ exchange: undefined, symbol: undefined });
    }
  }, [scope, form]);

  // 根据交易所加载交易对
  useEffect(() => {
    if (!selectedExchange || !open) {
      setSymbolOptions([]);
      form.setFieldsValue({ symbol: undefined });
      return;
    }

    setLoadingSymbols(true);
    api
      .queryMarkets({ exchange: selectedExchange })
      .then((markets) => {
        const options =
          markets?.map((market: { symbol: string }) => ({
            label: market.symbol,
            value: market.symbol,
          })) || [];
        setSymbolOptions(options);
      })
      .catch((error) => {
        console.error('Failed to load symbols:', error);
        setSymbolOptions([]);
      })
      .finally(() => {
        setLoadingSymbols(false);
      });
  }, [selectedExchange, open, form]);

  // 初始化表单值和 signalProps
  useEffect(() => {
    if (!open) {
      return;
    }

    if (value) {
      const propsFormValues = propsToFormValues(value.props);
      form.setFieldsValue({
        id: value.id || '',
        type: value.type,
        exchange: value.exchange,
        symbol: value.symbol,
        scope: value.scope,
        props: propsFormValues,
      });
    } else {
      form.resetFields();
      form.setFieldsValue({ id: '' });
    }
  }, [value, open, form]);

  const renderPropsField = (signalType: SignalType) => {
    switch (signalType) {
      case SignalType.Kline:
        return (
          <ProForm.Group>
            <ProFormSelect
              name={['props', 'interval']}
              label="时间间隔"
              options={KlineIntervalOptions}
              rules={[{ required: true, message: '请选择时间间隔' }]}
            />
          </ProForm.Group>
        );
      case SignalType.Timer:
        return (
          <ProForm.Group>
            <ProFormDigit
              name={['props', 'interval']}
              label="时间间隔（毫秒）"
              rules={[{ required: true, message: '请输入时间间隔' }]}
            />
            <ProFormText
              name={['props', 'topic']}
              label="主题"
              rules={[{ required: true, message: '请输入主题' }]}
            />
          </ProForm.Group>
        );
    }
    return null;
  };
  return (
    <ModalForm<any>
      form={form}
      title="信号定义"
      width="600px"
      style={{
        marginTop: 24,
      }}
      open={open}
      onOpenChange={onOpenChange}
      readonly={readonly}
      submitter={readonly ? false : undefined}
      modalProps={{
        centered: true,
        destroyOnHidden: true,
      }}
      clearOnDestroy
      onFinish={async (values) => {
        if (!onFinish) return true;
        const props = formValuesToProps(values.props);
        const formData: SignalDefinition = {
          id: values.id,
          type: values.type,
          exchange: values.exchange,
          symbol: values.symbol,
          scope: values.scope,
          props: props,
        };
        return await onFinish(formData);
      }}
    >
      <ProForm.Group>
        <ProFormText
          name="id"
          label="信号ID"
          readonly
          tooltip="信号ID将根据在列表中的排序自动生成"
        />
        <ProFormSelect
          name="type"
          label="类型"
          width={120}
          options={SignalTypeOptions}
          rules={[{ required: true, message: '请选择类型' }]}
        />
        <ProFormSelect
          name="scope"
          label="作用域"
          options={SignalScopeOptions}
          width={160}
          rules={[{ required: true, message: '请选择作用域' }]}
          tooltip={{
            styles: {
              body: {
                fontSize: 12,
                width: 260,
              },
            },
            placement: 'right',
            title: (
              <>
                <b>作用域分级：</b>
                <br />
                <b>Strategy 级别：</b>整个策略只需要一个数据源；
                <br />
                <b>Exchange 级别：</b>每个交易所只需要一个数据源；
                <br />
                <b>Symbol 级别：</b>每个交易对都需要独立的数据源；
                <br />
                <b>Target 级别：</b>针对策略中指定的具体 symbol（固定）；
              </>
            ),
          }}
          fieldProps={{
            placeholder: '请选择作用域',
          }}
        />
      </ProForm.Group>
      {scope === SignalScope.Target && (
        <ProForm.Group>
          <Form.Item
            name="exchange"
            label="交易所"
            style={{ width: 150 }}
            rules={[{ required: true, message: '请选择交易所' }]}
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
            rules={[{ required: true, message: '请选择交易对' }]}
          >
            <Select
              showSearch
              loading={loadingSymbols}
              options={symbolOptions}
              disabled={!selectedExchange || loadingSymbols}
              placeholder={selectedExchange ? '请选择交易对' : '请先选择交易所'}
              style={{
                width: 200,
              }}
              filterOption={(input: string, option?: { label: string; value: string }) => {
                if (!option) return false;
                const searchText = input.toUpperCase();
                const label = option.label?.toUpperCase() || '';
                return label.includes(searchText);
              }}
            />
          </ProFormField>
        </ProForm.Group>
      )}

      {renderPropsField(signalType)}
    </ModalForm>
  );
};

export default SignalModal;
