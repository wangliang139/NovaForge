import { Tag } from 'antd';
import React, { ReactNode } from 'react';

interface ReadonlyWrapperProps {
  /** 是否为只读模式 */
  readonly?: boolean;
  /** 子组件，仅支持单个子组件 */
  children: ReactNode;
  /** 是否隐藏 */
  hidden?: boolean;

  // 表单联动
  /** 表单项的唯一标识，用于支持跳转 scrollToField 方法 */
  id?: string;
  /** 当前值 */
  value?: any;
  /** 值变化回调 */
  onChange?: (value: any) => void;
}

const getFirstChild = (children: ReactNode) => {
  if (!children) {
    return null;
  }
  if (React.isValidElement(children)) {
    return children as React.ReactElement;
  }
  const childElements = React.Children.toArray(children);
  if (childElements.length > 0 && React.isValidElement(childElements[0])) {
    return childElements[0] as React.ReactElement;
  }
  return null;
};

// 处理只读模式下的显示
const getReadonlyValue = (child: ReactNode, value: any) => {
  // 如果子组件不存在，返回null
  if (!child || typeof child !== 'object' || React.isValidElement(child) === false) {
    return child;
  }

  const element = child as React.ReactElement;
  const { type, props } = element;
  const typeName =
    typeof type === 'string' ? type : (type as any).displayName || (type as any).name;

  // 根据不同的元素类型处理值的显示
  switch (typeName) {
    case 'input':
      // 处理输入框
      if (props.type === 'checkbox') {
        return props.checked ? '是' : '否';
      }
      if (props.type === 'radio') {
        return props.checked ? props.value : null;
      }
      return props.value ?? '';

    // case 'select':
    //   // 处理下拉选择器
    //   if (props.value === undefined) return '';

    //   // 查找选中的选项
    //   return React.Children.toArray(props.children).find(
    //     (option: any) => option.props.value === props.value
    //   )?.option?.props.children ?? props.value;

    case 'textarea':
      // 处理文本域
      return props.value ?? '';

    case 'Checkbox':
    case 'Switch':
      // 处理自定义复选框/开关组件
      return props.checked ? '是' : '否';

    case 'Radio':
      // 处理自定义单选按钮
      return props.checked ? props.children : null;

    case 'Select':
      let options = props.options || [];
      let mode = props.mode || 'default';

      // 查找选中的选项
      if (mode === 'tags' || mode === 'multiple') {
        return (
          <>
            {value.map((item: any, index: number) => (
              <Tag key={index}>
                {options.find((option: any) => option.value === item)?.label ?? item}
              </Tag>
            ))}
          </>
        );
      }
      return options.find((option: any) => option.value === value)?.label ?? value;

    default:
      // 对于未知组件，尝试查找value或children
      if ('value' in props) {
        return props.value;
      }
      return props.children ?? '';
  }
};

/**
 * 只读模式包装组件
 * 当readonly为true时，显示子组件的值
 * 当readonly为false时，显示原始子组件，如果提供了value则传递给子组件
 */
const ReadonlyWrapper: React.FC<ReadonlyWrapperProps> = ({
  readonly = false,
  hidden = false,
  value,
  id,
  children,
  onChange,
}) => {
  // 如果不是只读模式
  const child = getFirstChild(children);

  if (!readonly) {
    if (child) {
      return React.cloneElement(child, {
        id: id,
        value: value,
        hidden: hidden,
        onChange: onChange,
      });
    }
    return <>{children}</>;
  } else {
    const element = child as React.ReactElement;
    const { type, props } = element;
    const typeName =
      typeof type === 'string' ? type : (type as any).displayName || (type as any).name;

    // console.log('wrapper:', typeName, props, typeof value, value);

    // 只读模式：显示值
    let displayValue = value;
    if (
      displayValue === undefined ||
      displayValue === null ||
      displayValue === '' ||
      (Array.isArray(displayValue) && displayValue.length === 0)
    ) {
      displayValue = '-';
    } else if (typeof displayValue === 'boolean') {
      displayValue = displayValue ? '是' : '否';
    } else if (typeName === 'Select') {
      displayValue = getReadonlyValue(child, value);
    }
    return (
      <div id={id} hidden={hidden}>
        {displayValue}
      </div>
    );
  }
};

export default ReadonlyWrapper;
