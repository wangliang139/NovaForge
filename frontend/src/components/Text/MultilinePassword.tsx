import { Input } from 'antd';
import { useEffect, useRef, useState } from 'react';

type MultilinePasswordProps = {
  value?: string;
  onChange: (value: string) => void;
};

export default function MultiLinePassword(props: MultilinePasswordProps) {
  const { value, onChange } = props;
  const [realValue, setRealValue] = useState(value || '');
  const textAreaRef = useRef(null);

  useEffect(() => {
    setRealValue(value || '');
  }, [value]);

  const handleChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const maskedValue = e.target.value; // 全是 •

    // 根据光标前后的内容来推断用户输入的真实字符数
    const prevMasked = realValue.replace(/./g, '•');
    const minLen = Math.min(maskedValue.length, prevMasked.length);

    let diffPos = minLen;
    for (let i = 0; i < minLen; i++) {
      if (maskedValue[i] !== prevMasked[i]) {
        diffPos = i;
        break;
      }
    }

    if (maskedValue.length > prevMasked.length) {
      // 用户输入了新字符
      const addedCount = maskedValue.length - prevMasked.length;
      const added = '*'.repeat(addedCount); // placeholder，可替换为用户真实输入的字符
      setRealValue(realValue.slice(0, diffPos) + added + realValue.slice(diffPos));
    } else {
      // 删除
      const deleteCount = prevMasked.length - maskedValue.length;
      setRealValue(realValue.slice(0, diffPos) + realValue.slice(diffPos + deleteCount));
    }
    onChange(realValue);
  };

  return (
    <Input.TextArea
      ref={textAreaRef}
      value={realValue.replace(/./g, '•')}
      onChange={handleChange}
      autoSize={{ minRows: 3 }}
    />
  );
}
