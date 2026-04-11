import { ModalForm, ProForm, ProFormSelect, ProFormText } from '@ant-design/pro-components';
import { Form, Input, Typography } from 'antd';
import { TextAreaRef } from 'antd/es/input/TextArea';
import React, { useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import rehypeHighlight from 'rehype-highlight';
import remarkGfm from 'remark-gfm';
import { LlmMessage, LlmMessageRole } from '../types';

type PromptMessageModalProps = {
  open: boolean;
  value?: LlmMessage;
  onOpenChange: (open: boolean) => void;
  onFinish?: (formData: LlmMessage) => Promise<boolean | void> | void;
};

const PromptMessageModal: React.FC<PromptMessageModalProps> = ({
  open,
  value,
  onOpenChange,
  onFinish,
}) => {
  const [form] = Form.useForm();
  const inputRef = useRef<TextAreaRef | null>(null);
  const [showContentInput, setShowContentInput] = useState(false);

  const contentValue = Form.useWatch('content', form);

  return (
    <ModalForm<LlmMessage>
      form={form}
      title="Prompt Message"
      width="800px"
      style={{
        marginTop: 24,
      }}
      initialValues={value || { role: LlmMessageRole.USER }}
      open={open}
      onOpenChange={onOpenChange}
      modalProps={{
        centered: true,
        destroyOnHidden: true,
      }}
      clearOnDestroy
      onFinish={async (values) => {
        if (onFinish) {
          return await onFinish(values);
        }
        return true;
      }}
    >
      <ProForm.Group>
        <ProFormSelect
          name="role"
          label="Role"
          allowClear={false}
          rules={[{ required: true }]}
          options={Object.values(LlmMessageRole).map((role) => ({ label: role, value: role }))}
        />
        <ProFormText
          name="name"
          label="Name"
          rules={[
            { max: 50, message: '最多 50 个字符' },
            { pattern: /^[a-zA-Z0-9_]+$/, message: '只能包含大小写字母、数字和下划线' },
          ]}
          fieldProps={{
            count: {
              show: true,
              max: 50,
            },
          }}
        />
      </ProForm.Group>
      <Form.Item label="Content" required>
        <Form.Item name="content" rules={[{ required: true }]} noStyle>
          <Input.TextArea
            ref={inputRef}
            hidden={!showContentInput}
            size="large"
            // rows={8}
            style={{ height: 300 }}
            onBlur={() => {
              setTimeout(() => {
                setShowContentInput(false);
              }, 0);
            }}
          />
        </Form.Item>
        <Typography.Paragraph
          type="secondary"
          style={{
            border: '1px solid #d9d9d9',
            borderRadius: 6,
            padding: '8px 12px',
            height: 300,
            marginBottom: 0,
            cursor: 'pointer',
            overflow: 'auto',
          }}
          hidden={showContentInput}
          onClick={() => {
            setShowContentInput(true);
            setTimeout(() => {
              if (inputRef.current) {
                inputRef.current.focus();
                const length = inputRef.current.resizableTextArea?.textArea?.value?.length || 0;
                inputRef.current.resizableTextArea?.textArea?.setSelectionRange(length, length);
              }
            }, 0);
          }}
        >
          <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]}>
            {contentValue}
          </ReactMarkdown>
        </Typography.Paragraph>
      </Form.Item>
    </ModalForm>
  );
};

export default PromptMessageModal;
