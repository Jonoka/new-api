/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useState, useCallback } from 'react';
import { Button, Select, Typography, Popconfirm, Tag } from '@douyinfe/semi-ui';
import {
  IconPlus,
  IconDelete,
  IconChevronUp,
  IconChevronDown,
} from '@douyinfe/semi-icons';
import { GripVertical } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { reorderAutoGroupItems } from '../../../../helpers';

const { Text } = Typography;

let _idCounter = 0;
const uid = () => `ag_${++_idCounter}`;

function parseAutoGroups(str) {
  if (!str || !str.trim()) return [];
  try {
    const parsed = JSON.parse(str);
    if (!Array.isArray(parsed)) return [];
    return parsed
      .filter((item) => typeof item === 'string')
      .map((code) => ({ _id: uid(), code }));
  } catch {
    return [];
  }
}

function serializeAutoGroups(items) {
  const codes = items.map((item) => item.code).filter(Boolean);
  return codes.length === 0 ? '' : JSON.stringify(codes);
}

export default function AutoGroupList({ value, groupOptions = [], onChange }) {
  const { t } = useTranslation();

  const [items, setItems] = useState(() => parseAutoGroups(value));
  const [draggedItemId, setDraggedItemId] = useState(null);
  const [dragOverItemId, setDragOverItemId] = useState(null);
  const [dragOverPosition, setDragOverPosition] = useState('before');

  const emitChange = useCallback(
    (newItems) => {
      setItems(newItems);
      onChange?.(serializeAutoGroups(newItems));
    },
    [onChange],
  );

  const addItem = useCallback(() => {
    emitChange([...items, { _id: uid(), code: '' }]);
  }, [items, emitChange]);

  const removeItem = useCallback(
    (id) => {
      emitChange(items.filter((i) => i._id !== id));
    },
    [items, emitChange],
  );

  const updateItem = useCallback(
    (id, code) => {
      emitChange(items.map((i) => (i._id === id ? { ...i, code } : i)));
    },
    [items, emitChange],
  );

  const moveUp = useCallback(
    (index) => {
      if (index <= 0) return;
      const next = [...items];
      [next[index - 1], next[index]] = [next[index], next[index - 1]];
      emitChange(next);
    },
    [items, emitChange],
  );

  const moveDown = useCallback(
    (index) => {
      if (index >= items.length - 1) return;
      const next = [...items];
      [next[index], next[index + 1]] = [next[index + 1], next[index]];
      emitChange(next);
    },
    [items, emitChange],
  );

  const resetDragState = useCallback(() => {
    setDraggedItemId(null);
    setDragOverItemId(null);
    setDragOverPosition('before');
  }, []);

  const handleDragStart = useCallback((event, id) => {
    setDraggedItemId(id);
    event.dataTransfer.effectAllowed = 'move';
    event.dataTransfer.setData('text/plain', id);
  }, []);

  const handleDragOver = useCallback(
    (event, id) => {
      const sourceId =
        draggedItemId || event.dataTransfer.getData('text/plain');
      if (!sourceId || sourceId === id) {
        setDragOverItemId(null);
        return;
      }

      event.preventDefault();
      const rect = event.currentTarget.getBoundingClientRect();
      setDragOverItemId(id);
      setDragOverPosition(
        event.clientY - rect.top > rect.height / 2 ? 'after' : 'before',
      );
      event.dataTransfer.dropEffect = 'move';
    },
    [draggedItemId],
  );

  const handleDrop = useCallback(
    (event, targetId) => {
      event.preventDefault();
      const sourceId =
        draggedItemId || event.dataTransfer.getData('text/plain');
      const rect = event.currentTarget.getBoundingClientRect();
      const dropPosition =
        event.clientY - rect.top > rect.height / 2 ? 'after' : 'before';
      const nextItems = reorderAutoGroupItems(
        items,
        sourceId,
        targetId,
        dropPosition,
      );
      if (nextItems !== items) emitChange(nextItems);
      resetDragState();
    },
    [draggedItemId, emitChange, items, resetDragState],
  );

  if (items.length === 0) {
    return (
      <div>
        <Text type='tertiary' className='block text-center py-4'>
          {t('暂无自动分组，点击下方按钮添加')}
        </Text>
        <div className='mt-2 flex justify-center'>
          <Button icon={<IconPlus />} theme='outline' onClick={addItem}>
            {t('添加分组')}
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div>
      <div className='flex flex-col'>
        {items.map((item, index) => {
          const isDragging = draggedItemId === item._id;
          const isDropTarget =
            dragOverItemId === item._id && draggedItemId !== item._id;
          return (
            <div
              key={item._id}
              className='relative flex items-center gap-2 rounded-sm py-1'
              onDragOver={(event) => handleDragOver(event, item._id)}
              onDrop={(event) => handleDrop(event, item._id)}
              style={{
                opacity: isDragging ? 0.55 : 1,
                transition: 'opacity 0.15s',
              }}
            >
              {isDropTarget && (
                <span
                  aria-hidden='true'
                  style={{
                    position: 'absolute',
                    right: 0,
                    left: 0,
                    height: 2,
                    top: dragOverPosition === 'before' ? 0 : undefined,
                    bottom: dragOverPosition === 'after' ? 0 : undefined,
                    background: 'var(--semi-color-primary)',
                    pointerEvents: 'none',
                    zIndex: 1,
                  }}
                />
              )}
              <span
                draggable={items.length > 1}
                aria-hidden='true'
                title={t('排序')}
                onDragStart={(event) => handleDragStart(event, item._id)}
                onDragEnd={resetDragState}
                className={`inline-flex h-6 w-6 shrink-0 items-center justify-center text-[var(--semi-color-text-2)] ${items.length > 1 ? 'cursor-grab active:cursor-grabbing' : 'cursor-default opacity-40'}`}
              >
                <GripVertical size={16} />
              </span>
              <Tag size='small' color='blue' className='shrink-0'>
                {index + 1}
              </Tag>
              <Select
                size='small'
                filter
                value={item.code || undefined}
                placeholder={t('选择分组')}
                optionList={groupOptions}
                onChange={(v) => updateItem(item._id, v)}
                style={{ flex: 1 }}
                position='bottomLeft'
              />
              <Button
                icon={<IconChevronUp />}
                theme='borderless'
                size='small'
                disabled={index === 0}
                aria-label={`${t('排序')} ↑`}
                onClick={() => moveUp(index)}
              />
              <Button
                icon={<IconChevronDown />}
                theme='borderless'
                size='small'
                disabled={index === items.length - 1}
                aria-label={`${t('排序')} ↓`}
                onClick={() => moveDown(index)}
              />
              <Popconfirm
                title={t('确认移除？')}
                onConfirm={() => removeItem(item._id)}
                position='left'
              >
                <Button
                  icon={<IconDelete />}
                  type='danger'
                  theme='borderless'
                  size='small'
                />
              </Popconfirm>
            </div>
          );
        })}
      </div>
      <div className='mt-3 flex justify-center'>
        <Button icon={<IconPlus />} theme='outline' onClick={addItem}>
          {t('添加分组')}
        </Button>
      </div>
    </div>
  );
}
