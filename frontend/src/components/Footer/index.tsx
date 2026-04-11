import { Row, Typography } from 'antd';
import React from 'react';

const Footer: React.FC = () => {
  return (
    <Row
      justify="center"
      style={{
        background: 'none',
        padding: 10,
      }}
    >
      <Typography.Text>@ Powered by Ant Desgin</Typography.Text>
    </Row>
  );
};

export default Footer;
