import { CodeEditor } from '@/components';
import { Button, Col, Flex, message, Modal, Space } from 'antd';
import React, { useState } from 'react';

const toJsonSchema = require('to-json-schema');

type JsonSchemaGeneratorProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onFinish?: (jsonSchema: string) => Promise<boolean | void> | void;
};

const JsonSchemaGenerator: React.FC<JsonSchemaGeneratorProps> = ({
  open,
  onOpenChange,
  onFinish,
}) => {
  const [hasConverted, setHasConverted] = useState(false);

  const [jsonInput, setJsonInput] = useState<string>('');
  const [jsonSchema, setJsonSchema] = useState<string>('');

  const handleConvert = () => {
    try {
      if (!jsonInput || jsonInput.trim() === '') {
        message.warning('请输入 JSON 文本');
        return;
      }

      // 解析 JSON
      const json = JSON.parse(jsonInput);

      // 校验 JSON 格式
      // 判断 JSON 是否有任何属性
      if (
        !json ||
        typeof json !== 'object' ||
        Array.isArray(json) ||
        Object.keys(json).length === 0
      ) {
        message.warning('请输入合法的 JSON 文本');
        return;
      }

      // 转换为 JSON Schema
      const options = {
        // required: true,
        objects: {
          additionalProperties: false,
          postProcessFnc: (schema: any, obj: any, defaultFnc: any) => ({
            ...defaultFnc(schema, obj),
            $schema: 'https://json-schema.org/draft/2020-12/schema',
            required: Object.getOwnPropertyNames(obj),
          }),
        },
      };
      const schema = toJsonSchema(json, options);
      const schemaString = JSON.stringify(schema, null, 4);

      setJsonSchema(schemaString);
      setHasConverted(true);
    } catch (error: any) {
      console.error(error);
      message.error(`JSON 格式错误: ${error.message}`);
    }
  };

  const handleOpenChange = (open: boolean) => {
    if (!open) {
      // 关闭时重置状态
      setJsonSchema('');
      setJsonInput('');
      setHasConverted(false);
    }
    onOpenChange(open);
  };

  return (
    <Modal
      title="JSON Schema Generator"
      width="1000px"
      open={open}
      afterClose={() => {
        setJsonSchema('');
        setJsonInput('');
        setHasConverted(false);
      }}
      onCancel={() => handleOpenChange(false)}
      centered={true}
      destroyOnHidden={true}
      onOk={async () => {
        if (!hasConverted || !jsonSchema) {
          message.warning('请先完成 JSON 转换');
          return false;
        }
        if (onFinish) {
          return await onFinish(jsonSchema);
        }
        return true;
      }}
    >
      <Flex justify="center" align="center" gap={16}>
        <Col span={10}>
          <CodeEditor
            value={jsonInput}
            height="280px"
            onChange={(value) => setJsonInput(value)}
            placeholder="请输入 JSON 文本..."
          />
        </Col>
        <Space style={{ marginTop: 8 }}>
          <Button type="primary" onClick={handleConvert}>
            -&gt;
          </Button>
        </Space>
        <Col span={10}>
          <CodeEditor value={jsonSchema} height="280px" readonly />
        </Col>
      </Flex>
    </Modal>
  );
};

export default JsonSchemaGenerator;
