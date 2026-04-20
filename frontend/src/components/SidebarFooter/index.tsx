import { GithubOutlined, SendOutlined } from '@ant-design/icons';
import { Divider, Modal, Space, Typography } from 'antd';
import type { CSSProperties } from 'react';
import { useState } from 'react';
import pkg from '../../../package.json';

type SidebarFooterProps = {
  collapsed?: boolean;
};

const linkStyle: CSSProperties = {
  fontSize: 12,
  color: 'inherit',
  textDecoration: 'underline',
  opacity: 0.85,
};

const iconLinkStyle: CSSProperties = {
  color: '#56c6c9',
  fontSize: 16,
  lineHeight: 1,
};

const centerTextStyle: CSSProperties = {
  textAlign: 'center',
  color: 'inherit',
};

const appVersion = pkg.version;
const shortUserAgreementHtml = `
  <p>NovaForge 是一款 AI 辅助的数字资产交易分析工具，仅用于交流、学习与研究，不具备任何投资咨询资质。</p>
  <p>平台展示的分析结果、评分、回测数据及参考意见，均由系统基于历史数据或公开信息自动生成，仅供技术交流与学习参考，不构成任何形式的投资建议或交易决策依据。</p>
  <p>数字资产市场具有高波动性和不确定性，存在市场风险、流动性风险、技术风险及政策风险，可能导致本金部分或全部损失。您应结合自身风险承受能力、交易经验和财务状况，独立作出判断与决策，并自行承担由此产生的全部后果。</p>
  <p>您已知悉并同意：本产品尚未经充分测试与全面验证，不应被视为可直接用于实盘交易的成熟系统。若您自行将本产品用于实盘交易，由此造成的任何资产损失、收益损失或其他风险后果，均由您自行承担，与开发者无关。</p>
  <p><strong>市场有风险，决策需谨慎。</strong></p>
`;
const shortPrivacyPolicyHtml = `
  <p>我们重视您的隐私与数据保护。</p>
  <p>1) 收集范围：仅收集实现功能所需的信息（如邮箱、手机号、区号、Web3 钱包地址）以及必要的日志与设备信息。</p>
  <p>2) 使用目的：用于账户登录与安全校验、服务功能提供、问题排查与合规要求。</p>
  <p>3) 存储与安全：数据加密存储，并采取必要的权限与访问控制措施，尽力防止未经授权的访问、披露或丢失。</p>
  <p>4) 共享与第三方：除法律法规要求或履行服务所必需外，不会与第三方共享您的个人信息；若涉及第三方服务（如钱包、短信服务商），仅在实现功能所需的最小范围内处理。</p>
  <p>5) Cookies/本地存储：用于登录态与必要的会话维持（如令牌、PHPSESSID），您可在浏览器中进行清理或限制。</p>
  <p>6) 个人权利：您可根据法律法规行使查询、更正、删除、撤回同意等权利。</p>
  <p>7) 变更与通知：本条款更新后将在页面显著位置提示。继续使用本服务即表示您已阅读并同意更新内容。若您不同意本条款或其中任何更新，请停止使用本服务并联系我们。</p>
`;

const SidebarFooter = ({ collapsed }: SidebarFooterProps) => {
  const [agreementOpen, setAgreementOpen] = useState(false);
  const [privacyOpen, setPrivacyOpen] = useState(false);

  if (collapsed) return null;

  return (
    <div
      style={{
        padding: '10px 16px 0 16px',
      }}
    >
      <Typography.Paragraph style={{ ...centerTextStyle, marginBottom: 0, opacity: 0.65, fontSize: 12, fontWeight: 500 }}>
        社交账户
      </Typography.Paragraph>
      <Space
        size={16}
        style={{ ...centerTextStyle, width: '100%', justifyContent: 'center', marginTop: 4 }}
      >
        <a
          href="https://github.com/wangliang139/NovaForge"
          target="_blank"
          rel="noreferrer"
          aria-label="GitHub"
          style={iconLinkStyle}
        >
          <GithubOutlined />
        </a>
        <a
          href="https://t.me/NovaForge101"
          target="_blank"
          rel="noreferrer"
          aria-label="Telegram"
          style={iconLinkStyle}
        >
          <SendOutlined />
        </a>
      </Space>

      <Typography.Paragraph
        style={{ ...centerTextStyle, marginTop: 14, marginBottom: 0, fontSize: 12, opacity: 0.85 }}
      >
        <a
          href="#"
          style={linkStyle}
          onClick={(event) => {
            event.preventDefault();
            setAgreementOpen(true);
          }}
        >
          用户协议
        </a>{' '}
        <span style={{ opacity: 0.45 }}>&amp;</span>{' '}
        <a
          href="#"
          style={linkStyle}
          onClick={(event) => {
            event.preventDefault();
            setPrivacyOpen(true);
          }}
        >
          隐私条例
        </a>
      </Typography.Paragraph>

      <Divider style={{ margin: '14px 0 12px' }} />

      <Typography.Paragraph style={{ ...centerTextStyle, marginBottom: 4, opacity: 0.65, fontSize: 12 }}>
        © 2025-2026 NovaForge.
      </Typography.Paragraph>
      <Typography.Paragraph style={{ ...centerTextStyle, marginBottom: 4, opacity: 0.65, fontSize: 12 }}>
        All rights reserved.
      </Typography.Paragraph>
      <Typography.Paragraph style={{ ...centerTextStyle, marginBottom: 0, opacity: 0.5, fontSize: 12 }}>
        V{appVersion}
      </Typography.Paragraph>

      <Modal
        title="用户协议"
        open={agreementOpen}
        onCancel={() => setAgreementOpen(false)}
        footer={null}
        centered
      >
        <div
          style={{ lineHeight: 1.8 }}
          dangerouslySetInnerHTML={{ __html: shortUserAgreementHtml }}
        />
      </Modal>

      <Modal
        title="隐私条例"
        open={privacyOpen}
        onCancel={() => setPrivacyOpen(false)}
        footer={null}
        centered
      >
        <div
          style={{ lineHeight: 1.8 }}
          dangerouslySetInnerHTML={{ __html: shortPrivacyPolicyHtml }}
        />
      </Modal>
    </div>
  );
};

export default SidebarFooter;
