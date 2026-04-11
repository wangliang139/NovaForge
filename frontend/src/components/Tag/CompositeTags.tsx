import {
  closestCenter,
  DndContext,
  DragEndEvent,
  PointerSensor,
  useSensor,
  useSensors,
} from '@dnd-kit/core';
import { arrayMove, horizontalListSortingStrategy, SortableContext } from '@dnd-kit/sortable';
import { Flex } from 'antd';
import React, { useEffect, useState } from 'react';
import { EditableTag } from './EditableTag';
import { PlusInputTag } from './PlusInputTag';

type CompositeTagsProps = {
  value: string[];
  maxLength: number;
  readonly?: boolean;
  draggable?: boolean;
  style?: React.CSSProperties;
  plusTagLabel?: string;
  onChange?: (value: string[]) => void;
};

export const CompositeTags: React.FC<CompositeTagsProps> = (props) => {
  const { value, maxLength, readonly, draggable, style, plusTagLabel, onChange } = props;

  if (readonly && (!value || value.length === 0)) {
    return <div>-</div>;
  }

  const [tags, setTags] = useState<string[]>([]);
  const [editIndex, setEditIndex] = useState<number>(-1);

  const sensors = useSensors(useSensor(PointerSensor));

  useEffect(() => {
    setTags(value || []);
  }, [value]);

  const onTagsChange = (tags: string[]) => {
    setTags(tags);
    onChange?.(tags);
  };

  const handleAddTags = (value: string) => {
    if (value && !tags.includes(value)) {
      const newTags = [...tags, value];
      onTagsChange(newTags);
    }
  };

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over) {
      return;
    }
    if (active.id !== over.id) {
      const oldIndex = tags.findIndex((item) => item === active.id);
      const newIndex = tags.findIndex((item) => item === over.id);
      const newTags = arrayMove(tags, oldIndex, newIndex);
      onTagsChange(newTags);
    }
  };

  return (
    <>
      <DndContext sensors={sensors} onDragEnd={handleDragEnd} collisionDetection={closestCenter}>
        <SortableContext items={tags} strategy={horizontalListSortingStrategy}>
          <Flex gap="4px 0" wrap="wrap">
            {tags?.map<React.ReactNode>((value, index) => (
              <EditableTag
                id={value}
                key={value}
                value={value}
                style={style}
                status={index === editIndex ? 'edit' : 'show'}
                draggable={readonly ? false : draggable}
                readonly={readonly}
                onConfirm={(_: string, value: string) => {
                  tags[index] = value;
                  onTagsChange(tags);
                  setEditIndex(-1);
                }}
                onClose={(key: string) => {
                  onTagsChange(tags.filter((value) => value !== key));
                }}
                onTriggerEdit={() => {
                  setEditIndex(index);
                }}
              />
            ))}
            {!readonly && tags.length < maxLength && (
              <PlusInputTag
                label={plusTagLabel || 'New Tag'}
                status={editIndex == -1 ? 'show' : 'edit'}
                onConfirm={handleAddTags}
              />
            )}
          </Flex>
        </SortableContext>
      </DndContext>
    </>
  );
};
