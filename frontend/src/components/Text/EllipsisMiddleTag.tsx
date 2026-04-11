import { Typography } from 'antd';
import React from 'react';

const { Text } = Typography;

const EllipsisMiddleTag: React.FC<{ suffixCount: number; children: string }> = ({
  suffixCount,
  children,
}) => {
  const start = children.slice(0, children.length - suffixCount);
  const suffix = children.slice(-suffixCount).trim();
  return (
    <Text
      style={{
        maxWidth: '100%',
      }}
      ellipsis={{ suffix, tooltip: children }}
      code
      italic
      type="secondary"
    >
      {start}
    </Text>
  );
};

export default EllipsisMiddleTag;
