import { CodeEditor } from '@/components';
import { DocumentCatalog, DocumentCatalogOptions } from '@/services/gateway/document';
import { ExperimentOutlined } from '@ant-design/icons';
import { ModalForm, ProFormSelect, ProFormSwitch, ProFormText } from '@ant-design/pro-components';
import { Button, Col, Form, message, Row } from 'antd';
import React, { useEffect, useState } from 'react';
import { parseExtractCfgJson, type Channel, type ExtractCfg } from '@/services/gateway/document';
import ExtractTestModal from './ExtractTestModal';

interface ChannelModalProps {
  mode: 'new' | 'edit' | 'readonly';
  open: boolean;
  value?: Channel;
  onOpenChange: (open: boolean) => void;
  onFinish?: (value: Channel) => Promise<boolean | void> | void;
}

const defaultValues = {
  id: '',
  name: '',
  title: '',
  broadcast: false,
  source: '',
  catalog: DocumentCatalog.NEWS,
  extractCfg: { plans: [] },
  enabled: false,
};

const ChannelModal: React.FC<ChannelModalProps> = ({
  mode,
  open,
  value,
  onOpenChange,
  onFinish,
}) => {
  const [form] = Form.useForm();
  const [extractCfgText, setExtractCfgText] = useState<string>('{"plans":[]}');
  const [extractCfgError, setExtractCfgError] = useState<string>('');
  const [testModalOpen, setTestModalOpen] = useState(false);

  const isReadonly = mode === 'readonly';

  useEffect(() => {
    if (open) {
      const newValue = value || defaultValues;
      form.setFieldsValue({
        ...newValue,
      });
      const cfgStr = JSON.stringify(newValue.extractCfg || { plans: [] }, null, 2);
      setExtractCfgText(cfgStr);
      setExtractCfgError('');
    }
  }, [open, value, mode]);

  const handleExtractCfgBlur = () => {
    try {
      const checkResult = parseExtractCfgJson(extractCfgText);
      if (!checkResult.valid) {
        setExtractCfgError(`Invalid Extract Config: #${checkResult.field} ${checkResult.error}`);
        return;
      }
      setExtractCfgText(JSON.stringify(checkResult.data, null, 2)); // Re-format
      setExtractCfgError('');
    } catch (e) {
      setExtractCfgError('Invalid JSON format');
    }
  };

  const handleExtractCfgChange = (newExtractCfg: ExtractCfg) => {
    setExtractCfgText(JSON.stringify(newExtractCfg, null, 2));
    setExtractCfgError('');
  };

  const getTitle = () => {
    if (mode === 'new') return '新建 Channel';
    if (mode === 'edit') return '编辑 Channel';
    return '查看 Channel';
  };

  return (
    <>
      <ModalForm<Channel>
        title={getTitle()}
        open={open}
        form={form}
        onOpenChange={onOpenChange}
        width="800px"
        onFinish={async (values) => {
          if (extractCfgError) {
            message.error('请先修正 Extract Config 格式错误');
            return false;
          }
          let extractCfg: ExtractCfg;
          try {
            extractCfg = JSON.parse(extractCfgText);
          } catch (e) {
            message.error('Extract Config 格式错误');
            return false;
          }
          values.extractCfg = extractCfg;
          if (onFinish) {
            return await onFinish(values);
          }
          return true;
        }}
        submitter={{
          render: (props, defaultDoms) => {
            const buttons = [
              <Button
                key="test"
                icon={<ExperimentOutlined />}
                color="orange"
                variant="solid"
                onClick={() => setTestModalOpen(true)}
              >
                测试
              </Button>,
            ];
            if (!isReadonly) {
              buttons.push(...defaultDoms);
            }
            return buttons;
          },
        }}
      >
        <Row justify="space-between" style={{ width: '100%' }}>
          <Col span={12}>
            <ProFormText
              name="id"
              label="ID"
              colon={true}
              width="md"
              disabled={isReadonly || mode === 'edit'}
              rules={[{ required: true, message: '请输入 ID' }]}
            />
          </Col>
          <Col span={12}>
            <ProFormSwitch
              name="enabled"
              label="Enabled"
              colon={true}
              width="sm"
              rules={[{ required: true }]}
              disabled={isReadonly}
            />
          </Col>
        </Row>

        <Row justify="space-between" style={{ width: '100%' }}>
          <Col span={8}>
            <ProFormText
              name="name"
              label="Name"
              colon={true}
              width="sm"
              rules={[{ required: true, message: '请输入 Name' }]}
              disabled={isReadonly}
            />
          </Col>
          <Col span={8}>
            <ProFormText
              name="title"
              label="Title"
              colon={true}
              width="sm"
              rules={[{ required: true, message: '请输入 Title' }]}
              disabled={isReadonly}
            />
          </Col>
          <Col span={8}>
            <ProFormSwitch
              name="broadcast"
              label="Broadcast"
              colon={true}
              width="sm"
              rules={[{ required: true }]}
              disabled={isReadonly}
            />
          </Col>
        </Row>

        <Row justify="space-between" style={{ width: '100%' }}>
          <Col span={8}>
            <ProFormText
              name="source"
              label="Source"
              colon={true}
              width="sm"
              rules={[{ required: true, message: '请输入 Source' }]}
              disabled={isReadonly}
            />
          </Col>
          <Col span={8}>
            <ProFormSelect
              name="catalog"
              label="Catalog"
              colon={true}
              width="sm"
              options={DocumentCatalogOptions}
              rules={[{ required: true, message: '请选择 Catalog' }]}
              disabled={isReadonly}
            />
          </Col>
          <Col span={8}></Col>
        </Row>

        <Form.Item
          label="Extract Config (JSON)"
          required
          validateStatus={extractCfgError ? 'error' : ''}
          help={extractCfgError}
        >
          <CodeEditor
            value={extractCfgText}
            height="300px"
            readonly={isReadonly}
            onChange={(value) => setExtractCfgText(value)}
            onBlur={handleExtractCfgBlur}
            placeholder='输入 Extract Config，例如：{"plans":[]}'
          />
        </Form.Item>
      </ModalForm>

      <ExtractTestModal
        open={testModalOpen}
        value={extractCfgText}
        onOpenChange={setTestModalOpen}
        onExtractCfgChange={mode === 'edit' || mode === 'new' ? handleExtractCfgChange : undefined}
      />
    </>
  );
};

export default ChannelModal;
