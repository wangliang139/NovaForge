import { CodeEditor } from '@/components';
import { InteractionOutlined } from '@ant-design/icons';
import { Button, Card, Flex, Form, Input, Modal, Row, Space, message } from 'antd';
import React, { useEffect, useState } from 'react';
import { testExtract } from '@/services/gateway/document';
import useStyles from '../style.style';
import { ExtractCfg, ExtractTestResult, parseExtractCfgJson } from '@/services/gateway/document';

const { TextArea } = Input;

interface ExtractTestModalProps {
  open: boolean;
  value?: string;
  onOpenChange: (open: boolean) => void;
  onExtractCfgChange?: (extractCfg: ExtractCfg) => void;
}

const ExtractTestModal: React.FC<ExtractTestModalProps> = ({
  open,
  value,
  onOpenChange,
  onExtractCfgChange,
}) => {
  const { styles } = useStyles();

  const [form] = Form.useForm();
  const [testing, setTesting] = useState(false);
  const [extractCfgText, setExtractCfgText] = useState<string>('{"plans":[]}');
  const [result, setResult] = useState<ExtractTestResult | null>(null);

  const [checked, setChecked] = useState(false);
  const [extractCfgError, setExtractCfgError] = useState<string>('');

  useEffect(() => {
    if (open) {
      let initValue = value || '{"plans":[]}';
      setExtractCfgText(initValue);
      setResult(null);
      setExtractCfgError('');
      setChecked(false);
      form.resetFields();
    }
  }, [open, value]);

  const handleTest = async () => {
    try {
      const checkResult = parseExtractCfgJson(extractCfgText);
      if (!checkResult.valid) {
        setChecked(false);
        setExtractCfgError(`Invalid Extract Config: #${checkResult.field} ${checkResult.error}`);
        return;
      }
      setChecked(true);

      let parsedExtractCfg = checkResult.data;
      if (!parsedExtractCfg) {
        message.warning('Extract Config is empty');
        return;
      }

      const values = await form.validateFields();

      setTesting(true);
      const testResult = await testExtract({
        extractCfg: parsedExtractCfg,
        text: values.text,
      });
      setResult(testResult);
      message.success('测试提取成功');
    } catch (error: any) {
      message.error(error.message || '测试提取失败');
    } finally {
      setTesting(false);
    }
  };

  const handleExtractCfgBlur = () => {
    try {
      const checkResult = parseExtractCfgJson(extractCfgText);
      if (!checkResult.valid) {
        setChecked(false);
        setExtractCfgError(`Invalid Extract Config: #${checkResult.field} ${checkResult.error}`);
        return;
      }
      setChecked(true);
      setExtractCfgText(JSON.stringify(checkResult.data, null, 2)); // Re-format
      setExtractCfgError('');
    } catch (e) {
      setExtractCfgError('Invalid JSON format');
    }
  };

  const handleOk = () => {
    if (!checked) {
      message.error('Extract Config 格式错误');
      return;
    }
    if (onExtractCfgChange && extractCfgError === '') {
      try {
        const parsedExtractCfg = JSON.parse(extractCfgText);
        onExtractCfgChange(parsedExtractCfg);
      } catch (e) {
        // Do nothing, error already shown
      }
    }
    onOpenChange(false);
  };

  const handleCancel = () => {
    form.resetFields();
    onOpenChange(false);
  };

  return (
    <Modal
      title="测试提取"
      open={open}
      onOk={handleOk}
      onCancel={handleCancel}
      width={800}
      destroyOnHidden
      footer={(originNode, extra) => {
        return (
          <Flex justify="space-between">
            <Button
              key="extract"
              icon={<InteractionOutlined />}
              color="orange"
              variant="solid"
              onClick={handleTest}
              loading={testing}
            >
              提取
            </Button>
            <Space>{originNode}</Space>
          </Flex>
        );
      }}
    >
      <Space direction="vertical" style={{ width: '100%' }} size="large">
        <div>
          <Row style={{ marginBottom: 10 }}>
            <span className={styles.required}>
              <span style={{ fontWeight: 'bold' }}>Extract Config (JSON)</span>
            </span>
          </Row>
          <CodeEditor
            value={extractCfgText}
            height="300px"
            onChange={(value) => setExtractCfgText(value)}
            onBlur={handleExtractCfgBlur}
            placeholder='输入 Extract Config，例如：{"plans":[]}'
          />
          <div style={{ color: 'red', marginTop: 4 }} hidden={!extractCfgError}>
            {extractCfgError}
          </div>
        </div>

        <Form form={form} layout="vertical">
          <Form.Item
            name="text"
            label="测试文本"
            rules={[{ required: true, message: '请输入测试文本' }]}
          >
            <TextArea rows={6} placeholder="输入要测试的消息文本" />
          </Form.Item>
        </Form>

        {result && (
          <Card title="提取结果" size="small">
            <Space direction="vertical" style={{ width: '100%' }}>
              <div>
                <strong>Filtered:</strong> {result.filtered ? 'Yes' : 'No'}
              </div>
              <div>
                <strong>Hit Plan:</strong> {result.hitPlan || '-'}
              </div>
              {result.title && (
                <div>
                  <strong>Title:</strong> {result.title}
                </div>
              )}
              {result.content && (
                <div>
                  <strong>Content:</strong> {result.content}
                </div>
              )}
              {result.url && (
                <div>
                  <strong>URL:</strong> {result.url}
                </div>
              )}
              {result.publishedAt && (
                <div>
                  <strong>Published At:</strong>{' '}
                  {new Date(result.publishedAt * 1000).toLocaleString()}
                </div>
              )}
            </Space>
          </Card>
        )}
      </Space>
    </Modal>
  );
};

export default ExtractTestModal;
