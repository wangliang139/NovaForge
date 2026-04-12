import { CompositeTags } from '@/components';
import { Exchange } from '@/global.types';
import { Account, AccountType, AuthAlgorithm } from '@/services/gateway/account';
import { enumToOptions } from '@/utils/dict';
import { encrypt } from '@/utils/rsa';
import {
  ModalForm,
  ProForm,
  ProFormSelect,
  ProFormSwitch,
  ProFormText,
  ProFormTextArea,
} from '@ant-design/pro-components';
import { Form, Select } from 'antd';
import React, { useEffect, useState } from 'react';

interface AccountModalProps {
  mode: 'new' | 'edit' | 'readonly';
  open: boolean;
  value?: Account | null;
  onOpenChange: (open: boolean) => void;
  onFinish?: (formData: Account) => Promise<boolean | void> | void;
}

const defaultValues: Partial<Account> = {
  exchange: Exchange.Binance,
  algorithm: AuthAlgorithm.Hmac,
  accountType: AccountType.Real,
  tags: [],
  multiBotMode: false,
};

const AccountModal: React.FC<AccountModalProps> = ({
  mode,
  open,
  value,
  onOpenChange,
  onFinish,
}) => {
  const [form] = Form.useForm();
  const [tags, setTags] = useState<string[]>([]);
  const [isOkx, setIsOkx] = useState(false);

  const isReadonly = mode === 'readonly';
  const isVirtualSub = value?.accountType === AccountType.VirtualSub;
  const isReal = value?.accountType === AccountType.Real;

  useEffect(() => {
    if (!open) {
      return;
    }
    const nextValue = mode === 'new' ? defaultValues : value || {};
    form.setFieldsValue(nextValue);
    setTags([...(nextValue.tags || [])]);
    setIsOkx(nextValue.exchange === Exchange.OKX || nextValue.exchange === Exchange.OKXTest);
  }, [open, value, mode, form]);

  const onTagsChange = (nextTags: string[]) => {
    setTags(nextTags);
    form.setFieldsValue({ tags: nextTags });
  };

  const getTitle = () => {
    if (mode === 'new') return '新建账户';
    if (mode === 'edit') return '编辑账户';
    return '查看账户';
  };

  return (
    <ModalForm<Account>
      form={form}
      title={getTitle()}
      width="660px"
      grid={false}
      style={{
        paddingTop: 12,
        paddingLeft: 12,
        paddingRight: 12,
      }}
      modalProps={{
        destroyOnHidden: true,
      }}
      labelAlign="right"
      // layout="horizontal"
      open={open}
      onOpenChange={onOpenChange}
      submitter={isReadonly ? false : undefined}
      onFinish={async (formData: Account) => {
        if (!onFinish) {
          return true;
        }
        const success = await onFinish(formData);
        if (success === false) {
          return false;
        }
        setTags([]);
        return true;
      }}
    >
      {mode !== 'new' && <ProFormText name="id" label="ID" width="sm" readonly />}
      <ProForm.Group>
        <ProFormText
          name="name"
          label="Name"
          width="md"
          readonly={isReadonly}
          rules={[
            {
              required: true,
              message: 'Name is required',
            },
            {
              max: 50,
            },
          ]}
        />
        <ProFormSelect
          allowClear={false}
          name="exchange"
          label="Exchange"
          width="sm"
          readonly={isReadonly || mode !== 'new' || isVirtualSub}
          fieldProps={{
            options: enumToOptions(Exchange),
            onChange: (nextValue) => {
              setIsOkx(nextValue === Exchange.OKX || nextValue === Exchange.OKXTest);
            },
          }}
          rules={[
            {
              required: true,
              message: 'Exchange is required',
            },
          ]}
        />
      </ProForm.Group>
      <ProForm.Group>
        <ProFormSelect
          allowClear={false}
          name="accountType"
          label="账户类型"
          width="sm"
          readonly={isReadonly || mode !== 'new'}
          fieldProps={{
            options: isVirtualSub
              ? [{ label: '虚拟子账户', value: AccountType.VirtualSub }]
              : [{ label: '真实账户', value: AccountType.Real }],
          }}
          rules={[
            {
              required: true,
              message: '账户类型不能为空',
            },
          ]}
        />
        <ProFormSwitch
          name="multiBotMode"
          label="多 Bot 模式（子账户）"
          readonly={isReadonly || isVirtualSub}
          tooltip="启用后，每个实盘 Bot 使用独立虚拟子账户与初始资金池，仍共用本账户的交易所 API"
        />
      </ProForm.Group>
      {(isReal || mode === 'new') && (
        <>
          <ProFormSelect
            allowClear={false}
            name="algorithm"
            label="Algorithm"
            width="sm"
            readonly={isReadonly || isVirtualSub}
            fieldProps={{
              options: enumToOptions(AuthAlgorithm),
            }}
            rules={[
              {
                required: true,
                message: 'Algorithm is required',
              },
            ]}
          />
          <ProFormText
            name="apiKey"
            label="ApiKey"
            disabled={isReadonly || isVirtualSub}
            rules={[
              {
                required: true,
                message: 'ApiKey is required',
              },
            ]}
          />
          <ProFormTextArea
            name="apiSecret"
            label="ApiSecret"
            disabled={isReadonly || isVirtualSub}
            fieldProps={{
              onChange: (e) => {
                console.log('onChange', e);
                const value = e.target.value.trim();
                if (value.length === 0) {
                  form.setFieldsValue({ apiSecret: '' });
                  return;
                }
                form.setFieldsValue({ apiSecret: encrypt(value) });
              },
            }}
            rules={[
              {
                required: true,
                message: 'ApiSecret is required',
              },
            ]}
          />
          <ProFormText
            hidden={!isOkx}
            name="passphrase"
            label="Passphrase"
            width="md"
            disabled={isReadonly || isVirtualSub}
            fieldProps={{
              type: 'password',
            }}
            rules={[
              {
                required: isOkx,
                message: 'Passphrase is required',
              },
            ]}
          />
        </>
      )}
      <ProForm.Item
        name="tags"
        label="Tags"
        initialValue={tags}
        rules={[
          {
            validator: async (_: any, item: string[]) => {
              if (item && item.length > 10) {
                return Promise.reject(new Error('At most 10 tags'));
              }
            },
          },
        ]}
      >
        <Select mode="multiple" style={{ display: 'none' }} />
        <CompositeTags
          value={tags}
          maxLength={10}
          readonly={isReadonly}
          draggable={!isReadonly}
          onChange={onTagsChange}
        />
      </ProForm.Item>
    </ModalForm>
  );
};

export default AccountModal;
