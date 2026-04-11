import { enumToOptions } from '@/utils/dict';
import {
  ModalForm,
  ProForm,
  ProFormDigit,
  ProFormSelect,
  ProFormSwitch,
  ProFormText,
  ProFormTextArea,
} from '@ant-design/pro-components';
import { Form } from 'antd';
import React from 'react';
import { ParamType, StrategyParam } from '@/services/gateway/strategy';

type ParamModalProps = {
  open: boolean;
  value?: StrategyParam;
  readonly?: boolean;
  onOpenChange: (open: boolean) => void;
  onFinish?: (formData: StrategyParam) => Promise<boolean | void> | void;
};

const ParamModal: React.FC<ParamModalProps> = ({
  open,
  value,
  readonly,
  onOpenChange,
  onFinish,
}) => {
  const [form] = Form.useForm();
  const paramType = Form.useWatch('type', form);
  const prevTypeRef = React.useRef<string | undefined>(undefined);
  const isInitializedRef = React.useRef<boolean>(false);

  // 初始化时设置上一次的类型值
  React.useEffect(() => {
    if (open && value?.type && !isInitializedRef.current) {
      prevTypeRef.current = value.type;
      isInitializedRef.current = true;
    }
    if (!open) {
      isInitializedRef.current = false;
      prevTypeRef.current = undefined;
    }
  }, [open, value?.type]);

  // 处理初始值转换
  React.useEffect(() => {
    if (value?.default && paramType && isInitializedRef.current) {
      const currentDefault = form.getFieldValue('default');
      if (currentDefault !== value.default) {
        // 根据类型转换初始值
        if (paramType === ParamType.Number) {
          const numValue = parseFloat(value.default);
          if (!isNaN(numValue)) {
            form.setFieldValue('default', numValue);
          }
        } else {
          // 对于 Bool、Object、String 类型，直接使用字符串值
          form.setFieldValue('default', value.default);
        }
      }
    }
  }, [value, paramType, form]);

  // 监听类型变化，切换类型时清空默认值
  React.useEffect(() => {
    // 如果类型发生变化（不是初始化），清空默认值
    if (
      paramType &&
      prevTypeRef.current !== undefined &&
      prevTypeRef.current !== paramType &&
      isInitializedRef.current
    ) {
      form.setFieldValue('default', undefined);
      prevTypeRef.current = paramType;
    } else if (paramType && !prevTypeRef.current) {
      // 首次设置类型（新建时），记录类型但不清空
      prevTypeRef.current = paramType;
    }
  }, [paramType, form]);

  // 根据参数类型渲染不同的默认值输入组件
  const renderDefaultValueInput = () => {
    switch (paramType) {
      case ParamType.Number:
        return (
          <ProFormDigit
            name="default"
            label="默认值"
            placeholder="请输入数字"
            transform={(value: number | undefined) => ({
              default: value !== undefined && value !== null ? String(value) : undefined,
            })}
          />
        );
      case ParamType.Bool:
        return (
          <ProFormSelect
            name="default"
            label="默认值"
            options={[
              { label: '是', value: 'true' },
              { label: '否', value: 'false' },
            ]}
            placeholder="请选择"
          />
        );
      case ParamType.Object:
        return (
          <ProFormTextArea
            name="default"
            label="默认值（JSON）"
            placeholder='请输入 JSON 格式，例如: {"key": "value"}'
            fieldProps={{
              rows: 4,
            }}
            rules={[
              {
                validator: (_: any, value: string | undefined) => {
                  if (!value) {
                    return Promise.resolve();
                  }
                  try {
                    JSON.parse(value);
                    return Promise.resolve();
                  } catch (e) {
                    return Promise.reject(new Error('请输入有效的 JSON 格式'));
                  }
                },
              },
            ]}
          />
        );
      case ParamType.String:
      default:
        return (
          <ProFormTextArea
            name="default"
            label="默认值"
            placeholder="请输入字符串"
            fieldProps={{
              rows: 2,
            }}
          />
        );
    }
  };

  return (
    <ModalForm<StrategyParam>
      form={form}
      title="参数配置"
      width="600px"
      style={{
        marginTop: 24,
      }}
      omitNil={false}
      initialValues={value || { required: false, description: '' }}
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
        console.log(values);
        if (onFinish) {
          // 确保 default 字段总是被包含在提交数据中
          // 如果字段被清空（undefined、null 或空字符串），明确传递 undefined
          const defaultValue = values.default;
          const formData: StrategyParam = {
            ...values,
            // 明确处理 default 字段：空值统一转换为 undefined，确保后端能正确清空原有值
            default:
              defaultValue !== undefined &&
              defaultValue !== null &&
              defaultValue !== '' &&
              String(defaultValue).trim() !== ''
                ? String(defaultValue)
                : undefined,
          };
          return await onFinish(formData);
        }
        return true;
      }}
    >
      <ProForm.Group>
        <ProFormText
          name="name"
          label="参数名"
          rules={[{ required: true, message: '请输入参数名' }]}
        />
        <ProFormSelect
          name="type"
          label="类型"
          options={enumToOptions(ParamType)}
          rules={[{ required: true, message: '请选择类型' }]}
        />
        <ProFormSwitch name="required" label="必填" />
      </ProForm.Group>

      <ProFormTextArea name="description" label="描述" />

      {renderDefaultValueInput()}
    </ModalForm>
  );
};

export default ParamModal;
