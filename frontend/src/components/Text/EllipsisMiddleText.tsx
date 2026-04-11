import { Typography } from 'antd';
import React from 'react';

const { Text } = Typography;

type EllipsisMiddleTextProps = {
  suffixCount: number;
  children: string;
  className?: string;
  onClick?: () => void;
};

const EllipsisMiddleText: React.FC<EllipsisMiddleTextProps> = ({
  suffixCount,
  children,
  onClick,
  className,
}) => {
  const start = children.slice(0, children.length - suffixCount);
  const suffix = children.slice(-suffixCount).trim();
  return (
    <Text
      style={{ maxWidth: '100%' }}
      ellipsis={{ suffix, tooltip: children }}
      onClick={onClick}
      className={className}
    >
      {start}
    </Text>
  );
};

export default EllipsisMiddleText;
