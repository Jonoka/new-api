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
  Radio,
  Select,
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

const createInvoiceRequest = (type = 'personal', kind = 'normal') => ({
  ...createEmptyInvoiceRequest(type, kind),
  required: true,
});

const formatCny = (value) => `¥${Number(value || 0).toFixed(2)}`;

const getConfiguredPaymentMethods = (config) =>
  (Array.isArray(config?.pay_methods) ? config.pay_methods : []).filter(
    (method) => typeof method?.type === 'string' && method.type.trim(),
  );

const getConfiguredBepusdtChains = (config) =>
  (Array.isArray(config?.bepusdt_chains) ? config.bepusdt_chains : []).filter(
    (chain) => typeof chain?.trade_type === 'string' && chain.trade_type.trim(),
  );

const isPaymentProvider = (method, provider) =>
  method?.provider?.toLowerCase() === provider ||
  method?.type?.toLowerCase() === provider;

const isSafeHttpCheckoutUrl = (value) => {
  if (typeof value !== 'string' || !value.trim()) return false;
  try {
    const url = new URL(value.trim());
    return url.protocol === 'http:' || url.protocol === 'https:';
  } catch {
    return false;
  }
};

const isSafariBrowser = () =>
  navigator.userAgent.indexOf('Safari') > -1 &&
  navigator.userAgent.indexOf('Chrome') < 1;

const launchCheckout = (checkout) => {
  const url = checkout?.url?.trim();
  if (!isSafeHttpCheckoutUrl(url)) return false;

  if (checkout.type === 'redirect') {
    const paymentWindow = window.open(url, '_blank', 'noopener,noreferrer');
    if (!paymentWindow) return false;
    paymentWindow.opener = null;
    return true;
  }
  if (checkout.type !== 'form') return false;

  const form = document.createElement('form');
  form.action = url;
  form.method = 'POST';
  if (!isSafariBrowser()) form.target = '_blank';
  Object.entries(
    checkout.params && typeof checkout.params === 'object'
      ? checkout.params
      : {},
  ).forEach(([key, value]) => {
    const input = document.createElement('input');
    input.type = 'hidden';
    input.name = key;
    input.value = value == null ? '' : String(value);
    form.appendChild(input);
  });
  document.body.appendChild(form);
  form.submit();
  document.body.removeChild(form);
  return true;
};

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
  const [selectedPaymentType, setSelectedPaymentType] = useState('');
  const [selectedTradeType, setSelectedTradeType] = useState('');

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

  const paymentMethods = useMemo(
    () => getConfiguredPaymentMethods(config),
    [config],
  );
  const bepusdtChains = useMemo(
    () => getConfiguredBepusdtChains(config),
    [config],
  );
  const selectedPaymentMethod = useMemo(
    () =>
      paymentMethods.find((method) => method.type === selectedPaymentType) ||
      null,
    [paymentMethods, selectedPaymentType],
  );

  useEffect(() => {
    if (!visible) return undefined;
    let cancelled = false;

    const loadData = async () => {
      setLoading(true);
      setConfig(null);
      setSelectedRowKeys([]);
      setPreview(null);
      setPreviewError('');
      setSelectedPaymentType('');
      setSelectedTradeType('');
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
        const defaultKind = Array.isArray(nextConfig.kinds)
          ? nextConfig.kinds[0]
          : 'normal';
        const nextPaymentMethods = getConfiguredPaymentMethods(nextConfig);
        const nextBepusdtChains = getConfiguredBepusdtChains(nextConfig);
        setConfig(nextConfig);
        setOrders(nextOrders);
        setSelectedPaymentType(nextPaymentMethods[0]?.type || '');
        setSelectedTradeType(nextBepusdtChains[0]?.trade_type || '');
        setInvoice(
          createInvoiceRequest(
            defaultType || 'personal',
            defaultKind || 'normal',
          ),
        );
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
  }, [invoice.kind, invoice.type, selectedOrderRefs, t, visible]);

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

  const summary = preview || {};
  const invoiceFee = Math.max(0, Number(summary.invoice_fee) || 0);
  const paymentRequired = invoiceFee > 0;
  const balanceSelected = isPaymentProvider(selectedPaymentMethod, 'balance');
  const bepusdtSelected = isPaymentProvider(selectedPaymentMethod, 'bepusdt');

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
    if (paymentRequired && !selectedPaymentMethod) {
      Toast.warning({ content: t('请选择支付方式') });
      return;
    }
    if (paymentRequired && bepusdtSelected && !selectedTradeType) {
      Toast.warning({ content: t('请选择支付链') });
      return;
    }

    setSubmitting(true);
    try {
      const requestPayload = {
        orders: selectedOrderRefs,
        invoice,
      };
      const response =
        !paymentRequired || balanceSelected
          ? await API.post('/api/user/invoice/request', requestPayload)
          : await API.post('/api/user/invoice/payment', {
              ...requestPayload,
              payment_method: selectedPaymentMethod.type,
              ...(bepusdtSelected ? { trade_type: selectedTradeType } : {}),
            });
      if (!response.data?.success) {
        throw new Error(response.data?.message || t('提交开票申请失败'));
      }
      const result = response.data.data || {};
      if (!paymentRequired || balanceSelected || result.completed === true) {
        Toast.success({ content: t('开票申请已提交') });
        onSuccess?.();
        onCancel?.();
        return;
      }
      if (!launchCheckout(result.checkout)) {
        throw new Error(t('支付地址无效或不受支持'));
      }
      Toast.info({ content: t('支付页面已打开，开票申请待支付') });
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

  const paymentReady =
    !paymentRequired ||
    (!!selectedPaymentMethod && (!bepusdtSelected || !!selectedTradeType));
  const canSubmit =
    config?.enabled &&
    selectedOrderRefs.length > 0 &&
    !!preview &&
    !previewLoading &&
    paymentReady &&
    !submitting;

  const submitButtonText = !paymentRequired
    ? t('提交开票申请')
    : balanceSelected
      ? t('余额支付 {{amount}} 并申请开票', {
          amount: formatCny(invoiceFee),
        })
      : selectedPaymentMethod
        ? t('使用{{method}}支付 {{amount}}', {
            method: t(selectedPaymentMethod.name || selectedPaymentMethod.type),
            amount: formatCny(invoiceFee),
          })
        : t('请选择支付方式');

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
            {submitButtonText}
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
            <div className='grid grid-cols-1 gap-3 sm:grid-cols-3'>
              <div>
                <Text type='tertiary'>{t('已选订单')}</Text>
                <Title heading={6}>{selectedOrderRefs.length}</Title>
              </div>
              <div>
                <Text type='tertiary'>{t('历史订单已付金额')}</Text>
                <Title heading={6}>{formatCny(summary.order_amount)}</Title>
              </div>
              <div>
                <Text type='tertiary'>{t('本次应付开票服务费')}</Text>
                <Title heading={6}>{formatCny(summary.invoice_fee)}</Title>
              </div>
            </div>
            {preview && (
              <Banner
                className='mt-3'
                type='info'
                closeIcon={null}
                description={t(
                  '历史订单均已支付，本次只需支付开票服务费，不会重复收取订单金额。',
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

        {config?.enabled && preview && paymentRequired && (
          <Card title={t('支付开票服务费')} bodyStyle={{ padding: 16 }}>
            {paymentMethods.length > 0 ? (
              <div className='space-y-3'>
                <Radio.Group
                  type='button'
                  value={selectedPaymentType}
                  onChange={(event) =>
                    setSelectedPaymentType(event.target.value)
                  }
                >
                  {paymentMethods.map((method) => (
                    <Radio key={method.type} value={method.type}>
                      <span className='inline-flex items-center gap-2'>
                        <span
                          className='inline-block h-2 w-2 rounded-full'
                          style={{
                            backgroundColor:
                              method.color || 'var(--semi-color-primary)',
                          }}
                        />
                        {t(method.name || method.type)}
                      </span>
                    </Radio>
                  ))}
                </Radio.Group>
                {bepusdtSelected && (
                  <div className='flex items-center gap-3'>
                    <Text type='tertiary'>{t('支付网络')}</Text>
                    <Select
                      value={selectedTradeType}
                      onChange={setSelectedTradeType}
                      optionList={bepusdtChains.map((chain) => ({
                        value: chain.trade_type,
                        label: chain.name || chain.trade_type,
                      }))}
                      placeholder={t('请选择支付链')}
                      style={{ minWidth: 180 }}
                    />
                  </div>
                )}
              </div>
            ) : (
              <Banner
                type='warning'
                closeIcon={null}
                description={t('当前没有可用的开票支付方式')}
              />
            )}
          </Card>
        )}
      </div>
    </Modal>
  );
};

export default InvoiceBatchRequestModal;
