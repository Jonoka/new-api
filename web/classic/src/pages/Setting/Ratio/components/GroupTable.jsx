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

import React, { useState, useCallback, useMemo, useRef } from 'react';
import {
  Button,
  Input,
  InputNumber,
  Checkbox,
  Typography,
  Popconfirm,
} from '@douyinfe/semi-ui';
import {
  IconArrowRight,
  IconPlus,
  IconDelete,
  IconRefresh,
} from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import CardTable from '../../../../components/common/ui/CardTable';
import { createTemporaryGroupCode } from '../../../../helpers';

const { Text } = Typography;

let rowIdCounter = 0;
const createRowId = () => `gr_${++rowIdCounter}`;

const buildRows = (groups) =>
  (Array.isArray(groups) ? groups : []).map((group) => ({
    ...group,
    _rowId: group.id ? `group_${group.id}` : createRowId(),
  }));

const serializeRows = (rows) => rows.map(({ _rowId, ...group }) => group);

export default function GroupTable({
  groups,
  autoGroup,
  onChange,
  onAutoGroupChange,
  autoSelectableLocked = false,
  onMigrate,
  onMigrateCodes,
  disabled = false,
}) {
  const { t } = useTranslation();
  const [rows, setRows] = useState(() => buildRows(groups));
  const reservedCodesRef = useRef(
    new Set(
      (Array.isArray(groups) ? groups : [])
        .map((group) => String(group.code || '').trim())
        .filter(Boolean),
    ),
  );
  const displayRows = useMemo(
    () => [
      {
        _rowId: 'virtual_auto',
        _virtualAuto: true,
        id: null,
        name: t('自动选择') + ' (auto)',
        ratio: t('自动'),
        user_selectable: autoGroup?.user_selectable === true,
        exclusive: false,
        description: autoGroup?.description || '',
      },
      ...rows,
    ],
    [autoGroup, rows, t],
  );

  // 通过 ref 读取最新回调，避免输入时重建列定义导致光标跳动。
  const onChangeRef = useRef(onChange);
  onChangeRef.current = onChange;
  const autoGroupRef = useRef(autoGroup);
  autoGroupRef.current = autoGroup;
  const onAutoGroupChangeRef = useRef(onAutoGroupChange);
  onAutoGroupChangeRef.current = onAutoGroupChange;

  const emitAndSet = useCallback((updater) => {
    setRows((previousRows) => {
      const nextRows =
        typeof updater === 'function' ? updater(previousRows) : updater;
      onChangeRef.current?.(serializeRows(nextRows));
      return nextRows;
    });
  }, []);

  const updateRow = useCallback(
    (rowId, field, value) => {
      emitAndSet((previousRows) =>
        previousRows.map((row) => {
          if (row._rowId !== rowId) return row;
          return { ...row, [field]: value };
        }),
      );
    },
    [emitAndSet],
  );

  const addRow = useCallback(() => {
    emitAndSet((previousRows) => {
      const code = createTemporaryGroupCode(reservedCodesRef.current);
      reservedCodesRef.current.add(code);

      return [
        ...previousRows,
        {
          _rowId: createRowId(),
          id: null,
          code,
          name: '',
          description: '',
          ratio: 1,
          user_selectable: true,
          exclusive: false,
          status: 1,
          auto_enabled: false,
          auto_order: 0,
        },
      ];
    });
  }, [emitAndSet]);

  const removeRow = useCallback(
    (rowId) => {
      emitAndSet((previousRows) =>
        previousRows.filter((row) => row._rowId !== rowId),
      );
    },
    [emitAndSet],
  );

  const columns = useMemo(
    () => [
      {
        title: t('ID'),
        dataIndex: 'id',
        key: 'id',
        width: 72,
        render: (_, record) => (
          <Text type={record.id ? 'primary' : 'tertiary'}>
            {record.id || '-'}
          </Text>
        ),
      },
      {
        title: t('分组名称'),
        dataIndex: 'name',
        key: 'name',
        width: 180,
        render: (_, record) =>
          record._virtualAuto ? (
            <Text strong>{record.name}</Text>
          ) : (
            <Input
              size='small'
              value={record.name}
              disabled={disabled}
              placeholder={t('请输入分组名称')}
              onChange={(value) => updateRow(record._rowId, 'name', value)}
            />
          ),
      },
      {
        title: t('倍率'),
        dataIndex: 'ratio',
        key: 'ratio',
        width: 120,
        render: (_, record) =>
          record._virtualAuto ? (
            <Text type='tertiary'>{record.ratio}</Text>
          ) : (
            <InputNumber
              size='small'
              min={0}
              step={0.1}
              value={record.ratio}
              disabled={disabled}
              style={{ width: '100%' }}
              onChange={(value) =>
                updateRow(record._rowId, 'ratio', value ?? 0)
              }
            />
          ),
      },
      {
        title: t('用户可选'),
        dataIndex: 'user_selectable',
        key: 'user_selectable',
        width: 90,
        align: 'center',
        render: (_, record) => (
          <Checkbox
            checked={record.user_selectable}
            disabled={disabled || (record._virtualAuto && autoSelectableLocked)}
            onChange={(event) => {
              if (record._virtualAuto) {
                onAutoGroupChangeRef.current?.({
                  ...autoGroupRef.current,
                  user_selectable: event.target.checked,
                });
                return;
              }
              updateRow(record._rowId, 'user_selectable', event.target.checked);
            }}
          />
        ),
      },
      {
        title: t('独立分组'),
        dataIndex: 'exclusive',
        key: 'exclusive',
        width: 90,
        align: 'center',
        render: (_, record) =>
          record._virtualAuto ? (
            <Text type='tertiary'>-</Text>
          ) : (
            <Checkbox
              checked={record.exclusive === true}
              disabled={disabled}
              onChange={(event) => {
                const exclusive = event.target.checked;
                emitAndSet((previousRows) =>
                  previousRows.map((row) =>
                    row._rowId === record._rowId
                      ? {
                          ...row,
                          exclusive,
                          auto_enabled: exclusive ? false : row.auto_enabled,
                          auto_order: exclusive ? 0 : row.auto_order,
                        }
                      : row,
                  ),
                );
              }}
            />
          ),
      },
      {
        title: t('描述'),
        dataIndex: 'description',
        key: 'description',
        render: (_, record) =>
          record._virtualAuto || record.user_selectable ? (
            <Input
              size='small'
              value={record.description}
              disabled={disabled}
              placeholder={t('分组描述')}
              onChange={(value) => {
                if (record._virtualAuto) {
                  onAutoGroupChangeRef.current?.({
                    ...autoGroupRef.current,
                    description: value,
                  });
                  return;
                }
                updateRow(record._rowId, 'description', value);
              }}
            />
          ) : (
            <Text type='tertiary' size='small'>
              -
            </Text>
          ),
      },
      {
        title: '',
        key: 'actions',
        width: 50,
        render: (_, record) =>
          record._virtualAuto ? null : (
            <Popconfirm
              title={t(
                '确认标记删除该分组？保存时绑定令牌会自动切换为 auto；渠道、用户等其他引用仍会阻止删除。',
              )}
              onConfirm={() => removeRow(record._rowId)}
              position='left'
              disabled={disabled}
            >
              <Button
                icon={<IconDelete />}
                type='danger'
                theme='borderless'
                size='small'
                disabled={disabled}
              />
            </Popconfirm>
          ),
      },
    ],
    [autoSelectableLocked, disabled, emitAndSet, removeRow, t, updateRow],
  );

  return (
    <div>
      <CardTable
        columns={columns}
        dataSource={displayRows}
        rowKey='_rowId'
        hidePagination
        size='small'
        empty={<Text type='tertiary'>{t('暂无分组，点击下方按钮添加')}</Text>}
      />
      <div className='mt-3 flex flex-wrap justify-center gap-2'>
        <Button
          icon={<IconRefresh />}
          theme='outline'
          disabled={disabled}
          onClick={onMigrateCodes}
        >
          {t('迁移旧标识')}
        </Button>
        <Button
          icon={<IconArrowRight />}
          theme='outline'
          disabled={
            disabled || rows.filter((row) => Number(row.id) > 0).length < 1
          }
          onClick={onMigrate}
        >
          {t('迁移令牌')}
        </Button>
        <Button
          icon={<IconPlus />}
          theme='outline'
          disabled={disabled}
          onClick={addRow}
        >
          {t('添加分组')}
        </Button>
      </div>
    </div>
  );
}
