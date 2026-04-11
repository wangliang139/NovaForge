import { PlusOutlined } from '@ant-design/icons';
import { Input, Tag, theme } from 'antd';
import React, { useState } from 'react';

type PlusInputTagProps = {
  status?: 'edit' | 'show';
  label?: string;
  onConfirm: (value: string) => void;
};

export const TagInputStyle: React.CSSProperties = {
  width: 64,
  height: 22,
  marginInlineEnd: 8,
  verticalAlign: 'top',
};

export const PlusInputTag: React.FC<PlusInputTagProps> = (props) => {
  const { onConfirm, label } = props;
  const [status, setStatus] = useState<string>(props.status || 'show');
  const [value, setValue] = useState<string>('');

  const { token } = theme.useToken();
  const tagPlusStyle: React.CSSProperties = {
    height: 22,
    background: token.colorBgContainer,
    borderStyle: 'dashed',
  };

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setValue(e.target.value);
  };

  const handleInputConfirm = () => {
    onConfirm(value);
    setStatus('show');
    setValue('');
  };

  return status == 'edit' ? (
    <Input
      autoFocus={true}
      type="text"
      size="small"
      style={TagInputStyle}
      value={value}
      onChange={handleInputChange}
      onBlur={handleInputConfirm}
      onPressEnter={handleInputConfirm}
    />
  ) : (
    <Tag style={tagPlusStyle} icon={<PlusOutlined />} onClick={() => setStatus('edit')}>
      {label || 'New Tag'}
    </Tag>
  );
};
