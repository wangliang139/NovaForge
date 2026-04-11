import React from 'react';
import { Space } from 'antd';
import { WifiOutlined } from '@ant-design/icons';

type DelayMonitorProps = {
  delay?: number;
  simplify?: boolean | true;
};

const DelayMonitor = (props: DelayMonitorProps) => {
  const { delay, simplify } = props;
  let color = 'gray';
  if (delay != undefined) {
    color = delay < 3000 ? 'green' : delay < 10000 ? '#faad14' : 'red';
  }
  let width = simplify ? '30' : '150';
  return (
    <Space style={{ float: 'right', width: width + 'px' }}>
      <WifiOutlined style={{ color: color }} />
      {!simplify && <span>延迟(ms)：{delay != undefined ? delay : 0}</span>}
    </Space>
  );
};
export default DelayMonitor;
