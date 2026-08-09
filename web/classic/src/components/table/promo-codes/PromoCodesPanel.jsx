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

import React, { useMemo, useRef, useState } from 'react';
import {
  Button,
  Card,
  Empty,
  Form,
  Input,
  Modal,
  Pagination,
  SideSheet,
  Space,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { Plus, TicketPercent } from 'lucide-react';
import { usePromoCodesData, PROMO_CODE_STATUS } from '../../../hooks/promo-codes/usePromoCodesData';
import { getCurrencyConfig, timestamp2string } from '../../../helpers';
import {
  displayAmountToQuota,
  quotaToDisplayAmount,
} from '../../../helpers/quota';

const { Text } = Typography;

const defaultFormValues = {
  id: undefined,
  name: '',
  code: '',
  discount_type: 'percent',
  discount_value: 10,
  applies_to_topup: true,
  applies_to_all_subscription: false,
  subscription_plan_ids: [],
  max_redeem_count: 0,
  expired_time: null,
};

const isExpired = (record) =>
  record.expired_time > 0 && record.expired_time < Math.floor(Date.now() / 1000);

const formatDiscount = (record) => {
  if (record.discount_type === 'percent') {
    return `${record.discount_value}%`;
  }
  const amount = quotaToDisplayAmount(record.discount_value || 0);
  return `${getCurrencyConfig().symbol}${amount.toFixed(2)}`;
};

const parsePlanIds = (raw) =>
  String(raw || '')
    .split(',')
    .map((item) => Number(item.trim()))
    .filter((id) => Number.isFinite(id) && id > 0);

const toFormValues = (record) => ({
  id: record.id,
  name: record.name || '',
  code: record.code || '',
  discount_type: record.discount_type || 'percent',
  discount_value:
    record.discount_type === 'fixed'
      ? Number(quotaToDisplayAmount(record.discount_value || 0).toFixed(6))
      : Number(record.discount_value || 0),
  applies_to_topup: record.applies_to_topup === true,
  applies_to_all_subscription: record.applies_to_all_subscription === true,
  subscription_plan_ids: parsePlanIds(record.subscription_plan_ids),
  max_redeem_count: Number(record.max_redeem_count || 0),
  expired_time:
    record.expired_time > 0 ? new Date(record.expired_time * 1000) : null,
});

const buildPayload = (values) => ({
  id: values.id,
  name: String(values.name || '').trim(),
  code: String(values.code || '').trim().toUpperCase(),
  discount_type: values.discount_type || 'percent',
  discount_value:
    values.discount_type === 'fixed'
      ? displayAmountToQuota(values.discount_value)
      : Math.round(Number(values.discount_value || 0)),
  applies_to_topup: values.applies_to_topup === true,
  applies_to_all_subscription: values.applies_to_all_subscription === true,
  subscription_plan_ids: (values.subscription_plan_ids || []).join(','),
  max_redeem_count: Number(values.max_redeem_count || 0),
  expired_time: values.expired_time
    ? Math.floor(values.expired_time.getTime() / 1000)
    : 0,
});

const PromoCodesPanel = () => {
  const data = usePromoCodesData();
  const {
    t,
    promoCodes,
    plans,
    loading,
    searching,
    activePage,
    pageSize,
    total,
    searchKeyword,
    setSearchKeyword,
    searchPromoCodes,
    savePromoCode,
    updatePromoCodeStatus,
    deletePromoCode,
    handlePageChange,
    handlePageSizeChange,
  } = data;
  const [editorVisible, setEditorVisible] = useState(false);
  const [editingRecord, setEditingRecord] = useState(null);
  const formApiRef = useRef(null);

  const openCreate = () => {
    setEditingRecord(null);
    setEditorVisible(true);
    setTimeout(() => formApiRef.current?.setValues(defaultFormValues));
  };

  const openEdit = (record) => {
    setEditingRecord(record);
    setEditorVisible(true);
    setTimeout(() => formApiRef.current?.setValues(toFormValues(record)));
  };

  const submit = async (values) => {
    if (!values.name || !String(values.name).trim()) {
      return Promise.reject(t('请输入名称'));
    }
    if (!values.code || !String(values.code).trim()) {
      return Promise.reject(t('请输入优惠码'));
    }
    if (Number(values.discount_value || 0) <= 0) {
      return Promise.reject(t('优惠值必须大于 0'));
    }
    if (
      values.discount_type === 'percent' &&
      Number(values.discount_value || 0) > 100
    ) {
      return Promise.reject(t('优惠百分比不能超过 100'));
    }
    if (
      !values.applies_to_topup &&
      !values.applies_to_all_subscription &&
      (!values.subscription_plan_ids || values.subscription_plan_ids.length === 0)
    ) {
      return Promise.reject(t('优惠码必须至少指定一个适用范围'));
    }

    const payload = buildPayload(values);
    if (editingRecord?.id) {
      payload.id = editingRecord.id;
    }
    const ok = await savePromoCode(payload);
    if (ok) {
      setEditorVisible(false);
    }
  };

  const columns = useMemo(
    () => [
      { title: t('ID'), dataIndex: 'id', width: 80 },
      { title: t('名称'), dataIndex: 'name' },
      {
        title: t('优惠码'),
        dataIndex: 'code',
        render: (code) => <span className='font-mono'>{code}</span>,
      },
      {
        title: t('优惠'),
        dataIndex: 'discount_value',
        render: (_, record) => formatDiscount(record),
      },
      {
        title: t('适用范围'),
        dataIndex: 'scope',
        render: (_, record) => {
          const scopes = [];
          if (record.applies_to_topup) scopes.push(t('余额充值'));
          if (record.applies_to_all_subscription) scopes.push(t('全部订阅'));
          if (record.subscription_plan_ids) scopes.push(t('指定订阅'));
          return scopes.length ? scopes.join(' / ') : '-';
        },
      },
      {
        title: t('使用次数'),
        dataIndex: 'redeemed_count',
        render: (_, record) =>
          `${record.redeemed_count || 0}/${record.max_redeem_count || t('不限')}`,
      },
      {
        title: t('过期时间'),
        dataIndex: 'expired_time',
        render: (time) => (time ? timestamp2string(time) : t('永不过期')),
      },
      {
        title: t('状态'),
        dataIndex: 'status',
        render: (status, record) => {
          if (isExpired(record)) {
            return (
              <Tag color='orange' shape='circle'>
                {t('已过期')}
              </Tag>
            );
          }
          if (status === PROMO_CODE_STATUS.ENABLED) {
            return (
              <Tag color='green' shape='circle'>
                {t('已启用')}
              </Tag>
            );
          }
          if (status === PROMO_CODE_STATUS.USED) {
            return (
              <Tag color='grey' shape='circle'>
                {t('已用尽')}
              </Tag>
            );
          }
          return (
            <Tag color='red' shape='circle'>
              {t('已禁用')}
            </Tag>
          );
        },
      },
      {
        title: t('操作'),
        dataIndex: 'operate',
        fixed: 'right',
        render: (_, record) => (
          <Space wrap>
            <Button size='small' type='tertiary' onClick={() => openEdit(record)}>
              {t('编辑')}
            </Button>
            {record.status === PROMO_CODE_STATUS.ENABLED ? (
              <Button
                size='small'
                type='warning'
                onClick={() =>
                  updatePromoCodeStatus(record, PROMO_CODE_STATUS.DISABLED)
                }
              >
                {t('禁用')}
              </Button>
            ) : (
              <Button
                size='small'
                disabled={record.status === PROMO_CODE_STATUS.USED}
                onClick={() =>
                  updatePromoCodeStatus(record, PROMO_CODE_STATUS.ENABLED)
                }
              >
                {t('启用')}
              </Button>
            )}
            <Button
              size='small'
              type='danger'
              onClick={() =>
                Modal.confirm({
                  title: t('确定删除该优惠码？'),
                  content: t('此操作不可恢复'),
                  onOk: () => deletePromoCode(record),
                })
              }
            >
              {t('删除')}
            </Button>
          </Space>
        ),
      },
    ],
    [t],
  );

  return (
    <>
      <Card
        className='!rounded-2xl shadow-sm border-0'
        title={
          <div className='flex flex-col md:flex-row justify-between items-start md:items-center gap-2 w-full'>
            <Space>
              <TicketPercent size={16} />
              <Text>{t('优惠码管理')}</Text>
            </Space>
            <Space wrap>
              <Input
                value={searchKeyword}
                onChange={setSearchKeyword}
                placeholder={t('搜索优惠码')}
                showClear
                onEnterPress={() => searchPromoCodes(searchKeyword)}
              />
              <Button onClick={() => searchPromoCodes(searchKeyword)}>
                {t('搜索')}
              </Button>
              <Button type='primary' icon={<Plus size={14} />} onClick={openCreate}>
                {t('创建优惠码')}
              </Button>
            </Space>
          </div>
        }
        footer={
          <div className='flex justify-end'>
            <Pagination
              currentPage={activePage}
              pageSize={pageSize}
              total={total}
              showSizeChanger
              onPageChange={handlePageChange}
              onPageSizeChange={handlePageSizeChange}
            />
          </div>
        }
      >
        <Table
          rowKey='id'
          columns={columns}
          dataSource={promoCodes}
          loading={loading || searching}
          pagination={false}
          scroll={{ x: 980 }}
          empty={<Empty description={t('暂无优惠码')} />}
        />
      </Card>

      <SideSheet
        title={editingRecord ? t('更新优惠码') : t('创建优惠码')}
        visible={editorVisible}
        onCancel={() => setEditorVisible(false)}
        width={620}
        footer={
          <div className='flex justify-end gap-2'>
            <Button onClick={() => setEditorVisible(false)}>{t('取消')}</Button>
            <Button type='primary' onClick={() => formApiRef.current?.submitForm()}>
              {t('保存')}
            </Button>
          </div>
        }
      >
        <Form
          getFormApi={(api) => (formApiRef.current = api)}
          initValues={defaultFormValues}
          onSubmit={submit}
        >
          {({ values }) => (
            <>
              <Form.Input
                field='name'
                label={t('名称')}
                placeholder={t('请输入名称')}
                rules={[{ required: true, message: t('请输入名称') }]}
              />
              <Form.Input
                field='code'
                label={t('优惠码')}
                placeholder={t('请输入优惠码')}
                rules={[{ required: true, message: t('请输入优惠码') }]}
              />
              <Form.Select
                field='discount_type'
                label={t('优惠类型')}
                style={{ width: '100%' }}
              >
                <Form.Select.Option value='percent'>
                  {t('百分比')}
                </Form.Select.Option>
                <Form.Select.Option value='fixed'>
                  {t('固定额度')}
                </Form.Select.Option>
              </Form.Select>
              <Form.InputNumber
                field='discount_value'
                label={
                  values.discount_type === 'fixed'
                    ? t('固定优惠金额')
                    : t('优惠百分比')
                }
                min={0}
                max={values.discount_type === 'percent' ? 100 : undefined}
                prefix={
                  values.discount_type === 'fixed'
                    ? getCurrencyConfig().symbol
                    : undefined
                }
                suffix={values.discount_type === 'percent' ? '%' : undefined}
                style={{ width: '100%' }}
              />
              <Form.InputNumber
                field='max_redeem_count'
                label={t('使用次数上限')}
                min={0}
                extraText={t('0 表示不限')}
                style={{ width: '100%' }}
              />
              <Form.DatePicker
                field='expired_time'
                label={t('过期时间')}
                type='dateTime'
                placeholder={t('选择过期时间（可选，留空为永久）')}
                showClear
                style={{ width: '100%' }}
              />
              <Form.Checkbox field='applies_to_topup'>
                {t('适用于余额充值')}
              </Form.Checkbox>
              <Form.Checkbox field='applies_to_all_subscription'>
                {t('适用于全部订阅')}
              </Form.Checkbox>
              {!values.applies_to_all_subscription && (
                <Form.Select
                  field='subscription_plan_ids'
                  label={t('指定订阅套餐')}
                  multiple
                  optionList={(plans || []).map((plan) => ({
                    label: plan.title || `#${plan.id}`,
                    value: plan.id,
                  }))}
                  style={{ width: '100%' }}
                  placeholder={t('不选则不适用于指定订阅')}
                />
              )}
            </>
          )}
        </Form>
      </SideSheet>
    </>
  );
};

export default PromoCodesPanel;
