import { getTelegramSession, sendTelegramCode } from '@/pages/user/settings/components/news-source/api';
import { SendOutlined } from '@ant-design/icons';
import { Alert, Button, Card, Input, message, Space, Typography } from 'antd';
import React, { useState } from 'react';

const { Text } = Typography;

type Step = 'inputPhone' | 'inputCode' | 'success';

export type TelegramAuthToolProps = {
  appId: string;
  appHash: string;
  /** 拿到可用 Session 时回调（含登录成功后的 session 字符串） */
  onSessionObtained?: (session: string) => void;
  /** 为 true 时不包裹 Card，用于 Modal 内嵌 */
  embedded?: boolean;
};

const TelegramAuthTool: React.FC<TelegramAuthToolProps> = ({
  appId,
  appHash,
  onSessionObtained,
  embedded = false,
}) => {
  const [step, setStep] = useState<Step>('inputPhone');
  const [phoneNumber, setPhoneNumber] = useState<string>('');
  const [codeHash, setCodeHash] = useState<string>('');
  const [code, setCode] = useState<string>('');
  const [tempSession, setTempSession] = useState<string>('');
  const [session, setSession] = useState<string>('');
  const [loading, setLoading] = useState<boolean>(false);

  const handleSendCode = async () => {
    if (!appId || !appHash) {
      message.error('请输入 App ID 和 App Hash');
      return;
    }

    if (!phoneNumber) {
      message.error('请输入手机号');
      return;
    }

    if (!phoneNumber.startsWith('+')) {
      message.error('请输入正确的手机号格式，如 +8619900000001');
      return;
    }

    setLoading(true);
    try {
      const result = await sendTelegramCode(appId, appHash, phoneNumber);
      if (result.success && result.codeHash) {
        setCodeHash(result.codeHash);
        setTempSession(result.session || '');
        setStep('inputCode');
        message.success('验证码已发送');
      } else {
        message.error(result.message || '发送验证码失败');
      }
    } catch (error) {
      message.error('发送验证码失败，请检查网络连接');
    } finally {
      setLoading(false);
    }
  };

  const handleVerifyCode = async () => {
    if (!code) {
      message.error('请输入验证码');
      return;
    }

    if (!codeHash) {
      message.error('验证码已过期，请重新获取');
      setStep('inputPhone');
      return;
    }

    setLoading(true);
    try {
      const result = await getTelegramSession(
        appId,
        appHash,
        phoneNumber,
        codeHash,
        code,
        tempSession,
      );
      if (result.success && result.session) {
        if (onSessionObtained) {
          onSessionObtained(result.session);
          return;
        }
        setSession(result.session);
        setStep('success');
        message.success('授权成功');
      } else {
        message.error(result.message || '验证失败，请检查验证码是否正确');
      }
    } catch (error) {
      message.error('验证失败，请检查网络连接');
    } finally {
      setLoading(false);
    }
  };

  const handleBack = () => {
    setStep('inputPhone');
    setCode('');
    setCodeHash('');
    setTempSession('');
  };

  const handleReset = () => {
    setStep('inputPhone');
    setPhoneNumber('');
    setCode('');
    setCodeHash('');
    setTempSession('');
    setSession('');
  };

  const handleCopy = async () => {
    if (!session) {
      message.warning('暂无内容可复制');
      return;
    }
    try {
      await navigator.clipboard.writeText(session);
      message.success('已复制到剪贴板');
    } catch (err) {
      console.error(err);
      message.error('复制失败，请检查浏览器权限');
    }
  };

  const renderInputPhone = () => (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Text type="secondary">
        请先在主表单填写 App ID 与 App Hash，再使用与之一致的账号手机号登录。
      </Text>
      <div>
        <Text strong>手机号</Text>
        <Input
          placeholder="+8619900000001"
          value={phoneNumber}
          onChange={(e) => setPhoneNumber(e.target.value)}
          style={{ marginTop: 8 }}
          prefix={<SendOutlined />}
        />
        <Text type="secondary" style={{ display: 'block', marginTop: 4 }}>
          请输入带国家代码的手机号，例如 +8619900000001
        </Text>
      </div>
      <Button
        type="primary"
        onClick={handleSendCode}
        loading={loading}
        disabled={!appId?.trim() || !appHash?.trim() || !phoneNumber}
      >
        发送验证码
      </Button>
    </Space>
  );

  const renderInputCode = () => (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <div>
        <Text strong>手机号</Text>
        <Input value={phoneNumber} disabled style={{ marginTop: 8 }} />
      </div>
      <div>
        <Text strong>验证码</Text>
        <Input
          placeholder="请输入 6 位验证码"
          value={code}
          onChange={(e) => setCode(e.target.value)}
          style={{ marginTop: 8 }}
          maxLength={6}
        />
        <Text type="secondary" style={{ display: 'block', marginTop: 4 }}>
          请输入 Telegram 发送的 6 位验证码
        </Text>
      </div>
      <Space>
        <Button type="primary" onClick={handleVerifyCode} loading={loading} disabled={!code}>
          确认
        </Button>
        <Button onClick={handleBack} disabled={loading}>
          返回
        </Button>
      </Space>
    </Space>
  );

  const renderSuccess = () => (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Alert
        message="授权成功"
        description="已成功获取 Telegram Session，请妥善保管以下信息"
        type="success"
        showIcon
      />
      <div>
        <Text strong>Session</Text>
        <Input.TextArea
          value={session}
          readOnly
          rows={8}
          style={{ marginTop: 8, fontFamily: 'monospace' }}
        />
      </div>
      <Space>
        <Button type="primary" onClick={handleCopy}>
          复制 Session
        </Button>
        <Button onClick={handleReset}>重新授权</Button>
      </Space>
    </Space>
  );

  const body = (
    <>
      {step === 'inputPhone' && renderInputPhone()}
      {step === 'inputCode' && renderInputCode()}
      {step === 'success' && renderSuccess()}
    </>
  );

  if (embedded) {
    return body;
  }

  return (
    <Card
      title={
        <Space>
          <span>Telegram 账号授权</span>
        </Space>
      }
      variant="borderless"
      style={{ marginTop: 20 }}
    >
      {body}
    </Card>
  );
};

export default TelegramAuthTool;
