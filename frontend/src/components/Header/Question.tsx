import { QuestionCircleOutlined } from '@ant-design/icons';

export type SiderTheme = 'light' | 'dark';

export const Question = ({ height }: { height: number }) => {
  return (
    <div
      style={{
        display: 'flex',
        height: height || 26,
      }}
      onClick={() => {
        window.open('https://github.com/wangliang139/NovaForge');
      }}
    >
      <QuestionCircleOutlined />
    </div>
  );
};
