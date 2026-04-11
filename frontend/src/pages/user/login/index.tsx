import { Footer } from '@/components';
import { login as loginApi } from '@/services/ant-design-pro/api';
import { setAccessToken } from '@/utils/auth';
import { encrypt } from '@/utils/rsa';
import { LoginForm, ProFormText } from '@ant-design/pro-components';
import { Helmet } from '@umijs/max';
import { Alert, Button } from 'antd';
import { createStyles } from 'antd-style';
import React, { useState } from 'react';
import Settings from '../../../../config/defaultSettings';
import background from '@/assets/image/background.png';

const useStyles = createStyles(() => {
  return {
    container: {
      display: 'flex',
      flexDirection: 'column',
      height: '100vh',
      overflow: 'auto',
      backgroundImage: `url(${background})`,
      backgroundSize: '100% 100%',
    },
  };
});

const ErrorMessage: React.FC<{ content: string }> = ({ content }) => (
  <Alert style={{ marginBottom: 24 }} message={content} type="error" showIcon />
);

const Login: React.FC = () => {
  const [errorMsg, setErrorMsg] = useState<string>('');
  const { styles } = useStyles();

  return (
    <div className={styles.container}>
      <Helmet>
        <title>
          {'登录页'}- {Settings.title}
        </title>
      </Helmet>
      <div style={{ flex: '1', padding: '80px 0' }}>
        <LoginForm
          contentStyle={{
            minWidth: 280,
            maxWidth: '75vw',
          }}
          logo={<img alt="logo" src="/logo.svg" />}
          submitter={{
            render: () => {
              return <Button type="primary" size={'large'} htmlType="submit" style={{ width: '100%' }}>进入</Button>;
            },
          }}
          title="LLT Trade"
          subTitle="会当凌绝顶，一览众山小！"
          onFinish={async (values) => {
            setErrorMsg('');
            try {
              const res = await loginApi({
                password: encrypt(values.password),
              });
              const token = res?.data?.token;
              if (!token) {
                setErrorMsg('登录失败：未返回令牌');
                return;
              }
              setAccessToken(token);
              const urlParams = new URL(window.location.href).searchParams;
              const redirect = urlParams.get('redirect') || '/';
              window.location.href = decodeURIComponent(redirect);
            } catch {
              setErrorMsg('密码错误');
            }
          }}
        >
          {errorMsg && <ErrorMessage content={errorMsg} />}
          <ProFormText.Password
            name="password"
            fieldProps={{ size: 'large', autoComplete: 'current-password' }}
            placeholder="解锁密码"
            rules={[{ required: true, message: '请输入解锁密码' }]}
          />
        </LoginForm>
      </div>
      <Footer />
    </div>
  );
};

export default Login;
