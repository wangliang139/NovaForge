import { outLogin } from '@/services/ant-design-pro/api';
import { changeUserPassword } from '@/services/gateway/user';
import { clearAccessToken } from '@/utils/auth';
import { encrypt } from '@/utils/rsa';
import { List, Modal, Form, Input, message } from 'antd';
import { history, useModel } from '@umijs/max';
import React, { useState } from 'react';
import { flushSync } from 'react-dom';
import { stringify } from 'querystring';

const SecurityView: React.FC = () => {
  const [pwdOpen, setPwdOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [form] = Form.useForm<{ currentPassword: string; newPassword: string; confirm: string }>();
  const { setInitialState } = useModel('@@initialState');

  const data = [
    {
      title: '账户密码',
      description: '定期更换密码有助于保护账户安全。',
      actions: [
        <a key="modify" onClick={() => setPwdOpen(true)}>
          修改
        </a>,
      ],
    },
  ];

  return (
    <>
      <List
        itemLayout="horizontal"
        dataSource={data}
        renderItem={(item) => (
          <List.Item actions={item.actions}>
            <List.Item.Meta title={item.title} description={item.description} />
          </List.Item>
        )}
      />

      <Modal
        title="修改密码"
        open={pwdOpen}
        destroyOnClose
        confirmLoading={submitting}
        okText="保存"
        onCancel={() => {
          setPwdOpen(false);
          form.resetFields();
        }}
        onOk={async () => {
          try {
            const v = await form.validateFields();
            setSubmitting(true);
            await changeUserPassword({
              currentPassword: encrypt(v.currentPassword),
              newPassword: encrypt(v.newPassword),
            });
            clearAccessToken();
            try {
              await outLogin();
            } catch {
              // 与 AvatarDropdown 一致：远端失败不影响本地退出
            }
            flushSync(() => {
              setInitialState((s) => ({ ...s, currentUser: undefined }));
            });
            message.success('密码已更新，请使用新密码重新登录');
            setPwdOpen(false);
            form.resetFields();
            const { pathname, search } = window.location;
            history.replace({
              pathname: '/user/login',
              search: stringify({ redirect: pathname + search }),
            });
          } catch (e: unknown) {
            if (e && typeof e === 'object' && 'errorFields' in e) {
              return;
            }
            const err = e as { response?: { data?: { error?: string } }; message?: string };
            const msg = err?.response?.data?.error || err?.message || '修改失败';
            message.error(msg);
          } finally {
            setSubmitting(false);
          }
        }}
      >
        <Form form={form} layout="vertical" preserve={false}>
          <Form.Item
            name="currentPassword"
            label="当前密码"
            rules={[{ required: true, message: '请输入当前密码' }]}
          >
            <Input.Password autoComplete="current-password" />
          </Form.Item>
          <Form.Item
            name="newPassword"
            label="新密码"
            rules={[
              { required: true, message: '请输入新密码' },
              { min: 8, message: '至少 8 位字符' },
            ]}
            hasFeedback
          >
            <Input.Password autoComplete="new-password" />
          </Form.Item>
          <Form.Item
            name="confirm"
            label="确认新密码"
            dependencies={['newPassword']}
            hasFeedback
            rules={[
              { required: true, message: '请再次输入新密码' },
              ({ getFieldValue }) => ({
                validator(_, value) {
                  if (!value || getFieldValue('newPassword') === value) {
                    return Promise.resolve();
                  }
                  return Promise.reject(new Error('两次输入的密码不一致'));
                },
              }),
            ]}
          >
            <Input.Password autoComplete="new-password" />
          </Form.Item>
        </Form>
      </Modal>
    </>
  );
};

export default SecurityView;
