import { useSortable } from '@dnd-kit/sortable';
import { Input, InputRef, Tag, Tooltip } from 'antd';
import React, { useRef, useState } from 'react';

type EditableTagProps = {
  id: string;
  value: string;
  status: 'edit' | 'show';
  onConfirm: (id: string, value: string) => void;
  onClose: (id: string) => void;
  onTriggerEdit: (id: string) => void;

  readonly?: boolean;
  draggable?: boolean;
  style?: React.CSSProperties;
};

const tagInputStyle: React.CSSProperties = {
  width: 64,
  height: 22,
  marginInlineEnd: 8,
  verticalAlign: 'top',
};

export const EditableTag: React.FC<EditableTagProps> = (props) => {
  const { id, status, value, readonly, draggable, style, onConfirm, onClose, onTriggerEdit } =
    props;

  const inputRef = useRef<InputRef>(null);
  const [editInputValue, setEditInputValue] = useState(value);

  const { listeners, transform, transition, isDragging, setNodeRef } = useSortable({ id: id });

  let draggableStyle = {};
  if (status == 'show' && draggable) {
    draggableStyle = {
      cursor: 'move',
      transition: 'unset', // Prevent element from shaking after drag
    };
    if (transform) {
      draggableStyle = {
        ...draggableStyle,
        transform: `translate3d(${transform.x}px, ${transform.y}px, 0)`,
        transition: isDragging ? 'unset' : transition, // Improve performance/visual effect when dragging
      };
    }
  }

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setEditInputValue(e.target.value);
  };

  const handleInputConfirm = () => {
    // 合法性校验 todo
    let newValue = value;
    if (editInputValue != value) {
      newValue = editInputValue;
    }
    onConfirm(id, newValue);
  };

  const handleClose = () => {
    onClose(id);
  };

  const handleEditTrigger = (e: React.MouseEvent | React.KeyboardEvent) => {
    if (readonly) {
      return;
    }
    setEditInputValue(value);
    onTriggerEdit(id);
  };

  if (!readonly && status == 'edit') {
    return (
      <Input
        ref={inputRef}
        key={value}
        size="small"
        autoFocus={true} // 开启拖拽后自动聚焦功能异常修复 todo
        style={{ ...style, ...tagInputStyle, ...draggableStyle }}
        value={editInputValue}
        onChange={handleInputChange}
        onBlur={handleInputConfirm}
        onPressEnter={handleInputConfirm}
      />
    );
  }

  if (value.length > 20) {
    return (
      <Tag
        ref={draggable ? setNodeRef : undefined}
        key={value}
        closable={!readonly}
        style={{ userSelect: 'none', ...style, ...draggableStyle }}
        onClose={() => handleClose()}
      >
        <Tooltip title={value} key={value}>
          <span onDoubleClick={handleEditTrigger} {...(draggable ? listeners : {})}>
            {`${value.slice(0, 20)}...`}
          </span>
        </Tooltip>
      </Tag>
    );
  }
  return (
    <Tag
      ref={draggable ? setNodeRef : undefined}
      key={value}
      closable={!readonly}
      style={{ ...style, userSelect: 'none', ...draggableStyle }}
      onClose={() => handleClose()}
    >
      <span onDoubleClick={handleEditTrigger} {...(draggable ? listeners : {})}>
        {value}
      </span>
    </Tag>
  );
};
