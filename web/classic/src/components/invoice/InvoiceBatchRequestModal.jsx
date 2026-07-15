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
import {
  Banner,
  Button,
  Card,
  Modal,
  Space,
  Spin,
  Table,
  Tag,
  Toast,
  Typography,
} from '@douyinfe/semi-ui';
import { API, timestamp2string } from '../../helpers';
import InvoiceRequestForm, {
  createEmptyInvoiceRequest,
} from './InvoiceRequestForm';

const { Text, Title } = Typography;

const MAX_SELECTED_ORDERS = 100;

const orderKey = (order) => `${order.source_type}:${order.source_id}`;

const isOrderSelectable = (order) =>
  Boolean(order?.invoice_eligible) && !order?.invoiced;

const createInvoiceRequest = (type = 'personal') => ({
  ...createEmptyInvoiceRequest(),
  required: true,
  type,
});

const formatCny = (value) => `¥${Number(value || 0).toFixed(2)}`;

const getSourceLabel = (sourceType, t) =>
  sourceType === 'subscription' ? t('订阅购买') : t('余额充值');

const InvoiceBatchRequestModal = ({ visible, onCancel, onSuccess, t }) => {
  const [config, setConfig] = useState(null);
  const [orders, setOrders] = useState([]);
  const [selectedRowKeys, setSelectedRowKeys] = useState([]);
  const [invoice, setInvoice] = useState(createInvoiceRequest());
  const [preview, setPreview] = useState(null);
  const [previewError, setPreviewError] = useState('');
  const [loading, setLoading] = useState(false);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  const orderMap = useMemo(
    () => new Map(orders.map((order) => [orderKey(order), order])),
    [orders],
  );

  const selectedOrders = useMemo(
    () =>
      selectedRowKeys.map((key) => orderMap.get(key)).filter(isOrderSelectable),
    [orderMap, selectedRowKeys],
  );

  const selectedOrderRefs = useMemo(
    () =>
      selectedOrders.map((order) => ({
        source_type: order.source_type,
        source_id: order.source_id,
      })),
    [selectedOrders],
  );

  useEffect(() => {
    if (!visible) return undefined;
    let cancelled = false;

    const loadData = async () => {
      setLoading(true);
      setSelectedRowKeys([]);
      setPreview(null);
      setPreviewError('');
      try {
        const [configResponse, ordersResponse] = await Promise.all([
          API.get('/api/user/invoice/config'),
          API.get('/api/user/invoice/orders'),
        ]);
        if (cancelled) return;
        if (!configResponse.data?.success) {
          throw new Error(
            configResponse.data?.message || t('加载发票配置失败'),
          );
        }
        if (!ordersResponse.data?.success) {
          throw new Error(ordersResponse.data?.message || t('加载订单失败'));
        }
        const nextConfig = configResponse.data.data || {};
        const nextOrders = ordersResponse.data.data?.orders || [];
        const defaultType = Array.isArray(nextConfig.types)
          ? nextConfig.types[0]
          : 'personal';
        setConfig(nextConfig);
        setOrders(nextOrders);
        setInvoice(createInvoiceRequest(defaultType || 'personal'));
      } catch (error) {
        if (!cancelled) {
          Toast.error({ content: error.message || t('加载失败') });
          setOrders([]);
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    loadData();
    return () => {
      cancelled = true;
    };
  }, [t, visible]);

  useEffect(() => {
    if (!visible || selectedOrderRefs.length === 0) {
      setPreview(null);
      setPreviewError('');
      setPreviewLoading(false);
      return undefined;
    }

    let cancelled = false;
    setPreview(null);
    setPreviewError('');
    setPreviewLoading(true);
    const timer = window.setTimeout(async () => {
      try {
        const response = await API.post('/api/user/invoice/preview', {
          orders: selectedOrderRefs,
          invoice,
        });
        if (cancelled) return;
        if (!response.data?.success) {
          throw new Error(response.data?.message || t('计算开票费用失败'));
        }
        setPreview(response.data.data || null);
      } catch (error) {
        if (!cancelled) {
          setPreview(null);
          setPreviewError(error.message || t('计算开票费用失败'));
        }
      } finally {
        if (!cancelled) setPreviewLoading(false);
      }
    }, 200);

    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [invoice.type, selectedOrderRefs, t, visible]);

  const handleSelectionChange = (keys) => {
    const selectableKeys = keys.filter((key) =>
      isOrderSelectable(orderMap.get(key)),
    );
    if (selectableKeys.length > MAX_SELECTED_ORDERS) {
      Toast.warning({
        content: t('每次最多选择 {{count}} 个订单', {
          count: MAX_SELECTED_ORDERS,
        }),
      });
    }
    setSelectedRowKeys(selectableKeys.slice(0, MAX_SELECTED_ORDERS));
  };

  const handleSubmit = async () => {
    if (selectedOrderRefs.length === 0) {
      Toast.warning({ content: t('请至少选择一个可开票订单') });
      return;
    }
    if (!invoice.title?.trim()) {
      Toast.warning({ content: t('请填写发票抬头') });
      return;
    }
    if (invoice.type === 'company' && !invoice.tax_no?.trim()) {
      Toast.warning({ content: t('请填写纳税人识别号') });
      return;
    }
    if (!preview || previewLoading) {
      Toast.warning({ content: t('请等待开票费用计算完成') });
      return;
    }

    setSubmitting(true);
    try {
      const response = await API.post('/api/user/invoice/request', {
        orders: selectedOrderRefs,
        invoice,
      });
      if (!response.data?.success) {
        throw new Error(response.data?.message || t('提交开票申请失败'));
      }
      Toast.success({ content: t('开票申请已提交') });
      onSuccess?.();
      onCancel?.();
    } catch (error) {
      Toast.error({ content: error.message || t('提交开票申请失败') });
    } finally {
      setSubmitting(false);
    }
  };

  const columns = [
    {
      title: t('来源'),
      dataIndex: 'source_type',
      width: 100,
      render: (value) => getSourceLabel(value, t),
    },
    {
      title: t('订单号'),
      dataIndex: 'source_id',
      render: (value) => <Text copyable>{value}</Text>,
    },
    {
      title: t('支付方式'),
      dataIndex: 'payment_method',
      width: 110,
      render: (value) => value || '-',
    },
    {
      title: t('支付时间'),
      dataIndex: 'complete_time',
      width: 170,
      render: (value) => timestamp2string(value),
    },
    {
      title: t('实付金额'),
      dataIndex: 'paid_amount',
      width: 110,
      render: (value) => formatCny(value),
    },
    {
      title: t('开票状态'),
      dataIndex: 'invoiced',
      width: 110,
      render: (value, record) =>
        value ? (
          <Tag color='grey'>{t('已申请开票')}</Tag>
        ) : isOrderSelectable(record) ? (
          <Tag color='green'>{t('可开票')}</Tag>
        ) : (
          <Tag color='grey'>{t('不可开票')}</Tag>
        ),
    },
  ];

  const summary = preview || {};
  const canSubmit =
    config?.enabled &&
    selectedOrderRefs.length > 0 &&
    !!preview &&
    !previewLoading &&
    !submitting;

  return (
    <Modal
      title={t('申请开票')}
      visible={visible}
      onCancel={onCancel}
      size='large'
      keepDOM
      footer={
        <Space>
          <Button onClick={onCancel} disabled={submitting}>
            {t('取消')}
          </Button>
          <Button
            theme='solid'
            type='primary'
            loading={submitting}
            disabled={!canSubmit}
            onClick={handleSubmit}
          >
            {t('余额支付并申请开票')}
          </Button>
        </Space>
      }
    >
      <div className='space-y-4'>
        <Banner
          type='info'
          closeIcon={null}
          description={t(
            '请选择近 30 天内支付成功且从未申请过发票的订单。已开过的订单不能重复选择。',
          )}
        />

        {!loading && config && !config.enabled && (
          <Banner
            type='warning'
            closeIcon={null}
            description={t('当前不支持开发票')}
          />
        )}

        <Table
          columns={columns}
          dataSource={orders}
          rowKey={orderKey}
          loading={loading}
          size='small'
          scroll={{ x: 'max-content', y: 320 }}
          rowSelection={{
            selectedRowKeys,
            onChange: handleSelectionChange,
            getCheckboxProps: (record) => ({
              disabled: !isOrderSelectable(record),
              name: orderKey(record),
            }),
          }}
          pagination={
            orders.length > 8 ? { pageSize: 8, showSizeChanger: false } : false
          }
          empty={t('近 30 天暂无可展示的支付订单')}
        />

        <Spin spinning={previewLoading}>
          <Card bodyStyle={{ padding: 16 }}>
            <div className='grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4'>
              <div>
                <Text type='tertiary'>{t('已选订单')}</Text>
                <Title heading={6}>{selectedOrderRefs.length}</Title>
              </div>
              <div>
                <Text type='tertiary'>{t('订单实付合计')}</Text>
                <Title heading={6}>{formatCny(summary.order_amount)}</Title>
              </div>
              <div>
                <Text type='tertiary'>{t('开票服务费')}</Text>
                <Title heading={6}>{formatCny(summary.invoice_fee)}</Title>
              </div>
              <div>
                <Text type='tertiary'>{t('含服务费总计')}</Text>
                <Title heading={6}>
                  {formatCny(summary.invoice_total_amount)}
                </Title>
              </div>
            </div>
            {preview && (
              <Banner
                className='mt-3'
                type='warning'
                closeIcon={null}
                description={t(
                  '本次仅从账户余额扣除开票服务费，共 {{quota}} 额度。',
                  {
                    quota: Number(summary.fee_quota || 0),
                  },
                )}
              />
            )}
            {previewError && (
              <Banner
                className='mt-3'
                type='danger'
                closeIcon={null}
                description={previewError}
              />
            )}
          </Card>
        </Spin>

        {config?.enabled && (
          <Card title={t('发票资料')} bodyStyle={{ padding: 16 }}>
            <InvoiceRequestForm
              t={t}
              config={config}
              value={invoice}
              onChange={(nextInvoice) =>
                setInvoice({ ...nextInvoice, required: true })
              }
              invoiceFee={summary.invoice_fee || 0}
              showRequiredToggle={false}
            />
          </Card>
        )}
      </div>
    </Modal>
  );
};

export default InvoiceBatchRequestModal;
