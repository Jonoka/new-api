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

import React, { useCallback, useEffect, useState } from 'react';
import { Banner, Modal, Spin, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess, showWarning } from '../../../../helpers';

const { Text } = Typography;

export default function GroupCodeMigrationModal({
  visible,
  onCancel,
  onMigrated,
}) {
  const { t } = useTranslation();
  const [preview, setPreview] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [migrating, setMigrating] = useState(false);

  const loadPreview = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const response = await API.post('/api/group/code-migration/preview');
      if (!response || response.data?.success === false) {
        throw new Error(response?.data?.message || t('迁移预览失败'));
      }
      setPreview(response.data?.data || null);
    } catch (requestError) {
      setPreview(null);
      setError(requestError?.message || t('迁移预览失败'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    if (visible) {
      loadPreview();
    } else {
      setPreview(null);
      setError('');
    }
  }, [loadPreview, visible]);

  const handleMigrate = async () => {
    if (!preview?.can_execute || !preview?.groups?.length) return;
    setMigrating(true);
    try {
      const response = await API.post('/api/group/code-migration', {
        confirm: true,
      });
      if (!response || response.data?.success === false) {
        return showError(response?.data?.message || t('迁移失败'));
      }
      const summary = response.data?.data;
      if (summary?.warning) {
        showWarning(summary.warning);
      } else {
        showSuccess(
          t('已成功迁移 {{count}} 个分组标识', {
            count: summary?.groups?.length || 0,
          }),
        );
      }
      await onMigrated?.(summary);
      onCancel();
    } catch (requestError) {
      showError(requestError?.message || t('迁移失败'));
    } finally {
      setMigrating(false);
    }
  };

  const groups = Array.isArray(preview?.groups) ? preview.groups : [];
  const blockers = Array.isArray(preview?.blockers) ? preview.blockers : [];

  return (
    <Modal
      title={t('迁移旧分组标识')}
      visible={visible}
      onCancel={migrating ? undefined : onCancel}
      onOk={handleMigrate}
      okText={migrating ? t('迁移中...') : t('确认迁移')}
      cancelText={t('取消')}
      confirmLoading={migrating}
      cancelButtonProps={{ disabled: migrating }}
      okButtonProps={{
        disabled:
          loading ||
          Boolean(error) ||
          !preview?.can_execute ||
          groups.length === 0,
      }}
      maskClosable={!migrating}
      width={640}
    >
      <Banner
        type='warning'
        description={t(
          '执行前必须确保所有实例已升级到同一版本，并暂停分组、渠道、令牌、用户分组和订阅配置写入。',
        )}
        style={{ marginBottom: 16 }}
      />

      {loading && (
        <div className='flex items-center gap-2'>
          <Spin size='small' />
          <Text type='tertiary'>{t('正在预检分组标识迁移...')}</Text>
        </div>
      )}
      {error && <Text type='danger'>{error}</Text>}

      {preview && !loading && groups.length === 0 && (
        <Text type='tertiary'>{t('当前没有需要迁移的旧分组标识。')}</Text>
      )}

      {groups.length > 0 && (
        <>
          <Text strong>
            {t('将迁移 {{count}} 个分组标识', { count: groups.length })}
          </Text>
          <div className='mt-3 max-h-48 overflow-y-auto rounded border border-solid border-[var(--semi-color-border)]'>
            {groups.map((group) => (
              <div
                key={group.group_id}
                className='flex items-center justify-between gap-3 border-0 border-b border-solid border-[var(--semi-color-border)] px-3 py-2 last:border-b-0'
              >
                <span className='min-w-0 truncate'>
                  ID {group.group_id} · {group.name}
                </span>
                <code className='shrink-0'>
                  {group.old_code} → {group.target_code}
                </code>
              </div>
            ))}
          </div>
          <Text type='tertiary' style={{ display: 'block', marginTop: 12 }}>
            {t(
              '影响 {{channels}} 个渠道、{{tokens}} 个令牌、{{users}} 个用户、{{abilities}} 条能力记录、{{plans}} 个套餐和 {{subscriptions}} 条订阅记录。',
              {
                channels: preview.affected_channels || 0,
                tokens: preview.affected_tokens || 0,
                users: preview.affected_users || 0,
                abilities: preview.affected_abilities || 0,
                plans: preview.affected_subscription_plans || 0,
                subscriptions: preview.affected_subscriptions || 0,
              },
            )}
          </Text>
        </>
      )}

      {blockers.length > 0 && (
        <div className='mt-3 rounded border border-solid border-[var(--semi-color-danger-light-default)] bg-[var(--semi-color-danger-light-default)] px-3 py-2'>
          <Text type='danger' strong>
            {t('迁移已被以下冲突阻止：')}
          </Text>
          <ul className='mb-0 mt-2 pl-5'>
            {blockers.map((blocker) => (
              <li key={blocker}>
                <Text type='danger'>{blocker}</Text>
              </li>
            ))}
          </ul>
        </div>
      )}
    </Modal>
  );
}
