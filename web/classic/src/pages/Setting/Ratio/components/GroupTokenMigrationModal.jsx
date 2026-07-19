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

import React, { useEffect, useMemo, useState } from 'react';
import { Form, Modal, Select, Spin, Typography } from '@douyinfe/semi-ui';
import { IconArrowRight } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess, showWarning } from '../../../../helpers';

const { Text } = Typography;

function groupLabel(group) {
  return `ID ${group.id} · ${group.name || group.code}`;
}

export default function GroupTokenMigrationModal({
  visible,
  groups,
  onCancel,
  onMigrated,
}) {
  const { t } = useTranslation();
  const [sourceGroupId, setSourceGroupId] = useState('');
  const [targetGroupId, setTargetGroupId] = useState('');
  const [preview, setPreview] = useState(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [previewError, setPreviewError] = useState('');
  const [migrating, setMigrating] = useState(false);

  const persistedGroups = useMemo(
    () => (Array.isArray(groups) ? groups : []).filter((group) => group.id > 0),
    [groups],
  );
  const targetGroups = useMemo(
    () =>
      persistedGroups.filter(
        (group) =>
          Number(group.status) === 1 &&
          String(group.id) !== String(sourceGroupId),
      ),
    [persistedGroups, sourceGroupId],
  );
  const request = useMemo(() => {
    const source = Number(sourceGroupId);
    const target = Number(targetGroupId);
    if (source <= 0 || target <= 0 || source === target) return null;
    return { source_group_id: source, target_group_id: target };
  }, [sourceGroupId, targetGroupId]);

  useEffect(() => {
    if (!visible) {
      setSourceGroupId('');
      setTargetGroupId('');
      setPreview(null);
      setPreviewError('');
    }
  }, [visible]);

  useEffect(() => {
    if (
      targetGroupId &&
      !targetGroups.some((group) => String(group.id) === String(targetGroupId))
    ) {
      setTargetGroupId('');
    }
  }, [targetGroupId, targetGroups]);

  useEffect(() => {
    let active = true;
    setPreview(null);
    setPreviewError('');
    if (!visible || !request) {
      setPreviewLoading(false);
      return () => {
        active = false;
      };
    }

    setPreviewLoading(true);
    API.post('/api/group/token-migration/preview', request)
      .then((response) => {
        if (!active) return;
        if (!response || response.data?.success === false) {
          throw new Error(response?.data?.message || t('迁移预览失败'));
        }
        setPreview(response.data?.data || null);
      })
      .catch((error) => {
        if (active) setPreviewError(error?.message || t('迁移预览失败'));
      })
      .finally(() => {
        if (active) setPreviewLoading(false);
      });

    return () => {
      active = false;
    };
  }, [request, t, visible]);

  const handleMigrate = async () => {
    if (!request) return;
    setMigrating(true);
    try {
      const response = await API.post('/api/group/token-migration', request);
      if (!response || response.data?.success === false) {
        return showError(response?.data?.message || t('迁移失败'));
      }
      const summary = response.data?.data;
      if (summary?.warning) {
        showWarning(summary.warning);
      } else {
        showSuccess(
          t('已成功迁移 {{count}} 个令牌', {
            count: summary?.migrated_tokens || 0,
          }),
        );
      }
      onMigrated?.(summary);
      onCancel();
    } catch (error) {
      showError(error?.message || t('迁移失败'));
    } finally {
      setMigrating(false);
    }
  };

  return (
    <Modal
      title={t('迁移令牌分组')}
      visible={visible}
      onCancel={migrating ? undefined : onCancel}
      onOk={handleMigrate}
      okText={migrating ? t('迁移中...') : t('迁移令牌')}
      cancelText={t('取消')}
      confirmLoading={migrating}
      cancelButtonProps={{ disabled: migrating }}
      okButtonProps={{
        disabled: !request || previewLoading || Boolean(previewError),
      }}
      maskClosable={!migrating}
    >
      <Text type='tertiary'>
        {t(
          '所有明确绑定源分组的令牌都会迁移到目标分组，自动分组和继承分组不受影响。',
        )}
      </Text>

      <Form layout='vertical' style={{ marginTop: 16 }}>
        <div className='grid grid-cols-1 sm:grid-cols-[1fr_auto_1fr] gap-3 items-end'>
          <Form.Slot label={t('源分组')}>
            <Select
              value={sourceGroupId || undefined}
              placeholder={t('选择源分组')}
              style={{ width: '100%' }}
              disabled={migrating}
              onChange={(value) => setSourceGroupId(String(value || ''))}
            >
              {persistedGroups.map((group) => (
                <Select.Option key={group.id} value={String(group.id)}>
                  {groupLabel(group)}
                </Select.Option>
              ))}
            </Select>
          </Form.Slot>

          <IconArrowRight className='hidden sm:block mb-2' />

          <Form.Slot label={t('迁移目标分组')}>
            <Select
              value={targetGroupId || undefined}
              placeholder={t('选择目标分组')}
              style={{ width: '100%' }}
              disabled={!sourceGroupId || migrating}
              onChange={(value) => setTargetGroupId(String(value || ''))}
            >
              {targetGroups.map((group) => (
                <Select.Option key={group.id} value={String(group.id)}>
                  {groupLabel(group)}
                </Select.Option>
              ))}
            </Select>
          </Form.Slot>
        </div>
      </Form>

      {previewLoading && (
        <div className='flex items-center gap-2 mt-3'>
          <Spin size='small' />
          <Text type='tertiary'>{t('正在统计受影响令牌...')}</Text>
        </div>
      )}
      {previewError && (
        <Text type='danger' style={{ display: 'block', marginTop: 12 }}>
          {previewError}
        </Text>
      )}
      {preview && !previewLoading && (
        <div className='rounded border border-solid border-[var(--semi-color-border)] bg-[var(--semi-color-fill-0)] px-3 py-2 mt-3'>
          <Text strong>
            {t('将迁移 {{count}} 个令牌', {
              count: preview.migrated_tokens,
            })}
          </Text>
          <Text type='tertiary' style={{ display: 'block', marginTop: 4 }}>
            {t('影响 {{users}} 个用户，并移除 {{duplicates}} 个重复绑定', {
              users: preview.affected_users,
              duplicates: preview.deduplicated_tokens,
            })}
          </Text>
          {preview.cleaned_deleted_tokens > 0 && (
            <Text type='tertiary' style={{ display: 'block', marginTop: 4 }}>
              {t('同时清理 {{count}} 个已删除令牌的历史分组引用', {
                count: preview.cleaned_deleted_tokens,
              })}
            </Text>
          )}
        </div>
      )}
    </Modal>
  );
}
