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
  Button,
  Card,
  Form,
  Modal,
  Select,
  Space,
  Table,
  Tag,
  TextArea,
  Toast,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, timestamp2string } from '../../helpers';
import { isAdmin } from '../../helpers/utils';
import InvoiceBatchRequestModal from '../../components/invoice/InvoiceBatchRequestModal';

const { Text } = Typography;

const STATUS_OPTIONS = [
  { label: '待支付', value: 'payment_pending', color: 'orange' },
  { label: '待开票', value: 'pending', color: 'orange' },
  { label: '已开票', value: 'issued', color: 'green' },
  { label: '已关闭', value: 'closed', color: 'grey' },
];
const EDIT_STATUS_OPTIONS = STATUS_OPTIONS.filter(
  (item) => item.value !== 'payment_pending',
);

const SOURCE_LABEL = {
  topup: '余额充值',
  subscription: '订阅购买',
  batch: '合并订单',
};

const TYPE_LABEL = {
  personal: '对私',
  company: '对公',
};

const KIND_LABEL = {
  normal: '增值税普通发票',
  special: '增值税专用发票',
};

const InvoiceCenter = ({ adminOnly = false }) => {
  const { t } = useTranslation();
  const userIsAdmin = useMemo(() => isAdmin(), []);
  const adminView = adminOnly && userIsAdmin;
  const [records, setRecords] = useState([]);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [total, setTotal] = useState(0);
  const [status, setStatus] = useState('');
  const [editing, setEditing] = useState(null);
  const [formValues, setFormValues] = useState({
    status: 'issued',
    download_url: '',
    admin_remark: '',
  });
  const [saving, setSaving] = useState(false);
  const [requestVisible, setRequestVisible] = useState(false);

  const loadInvoices = async () => {
    setLoading(true);
    try {
      const base = adminView ? '/api/user/invoice' : '/api/user/invoice/self';
      const params = new URLSearchParams({
        p: String(page),
        page_size: String(pageSize),
      });
      if (adminView && status) {
        params.set('status', status);
      }
      const res = await API.get(`${base}?${params.toString()}`);
      if (res.data?.success) {
        setRecords(res.data.data?.items || []);
        setTotal(res.data.data?.total || 0);
      } else {
        Toast.error({ content: res.data?.message || t('加载失败') });
      }
    } catch (error) {
      Toast.error({ content: t('加载失败') });
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadInvoices();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [adminView, page, pageSize, status]);

  const openEdit = (record) => {
    setEditing(record);
    setFormValues({
      status:
        record.status === 'payment_pending'
          ? 'closed'
          : record.status || 'issued',
      download_url: record.download_url || '',
      admin_remark: record.admin_remark || '',
    });
  };

  const cancelInvoicePayment = async (record) => {
    if (!window.confirm(t('取消待支付开票申请前，请确认你尚未完成支付。'))) {
      return;
    }
    try {
      const response = await API.post(
        `/api/user/invoice/payment/${encodeURIComponent(record.source_id)}/cancel`,
      );
      if (!response.data?.success) {
        throw new Error(response.data?.message || t('取消开票申请失败'));
      }
      Toast.success({ content: t('待支付申请已取消') });
      await loadInvoices();
    } catch (error) {
      Toast.error({ content: error.message || t('取消开票申请失败') });
    }
  };

  const saveInvoice = async () => {
    if (!editing) return;
    setSaving(true);
    try {
      const res = await API.put(`/api/user/invoice/${editing.id}`, formValues);
      if (res.data?.success) {
        Toast.success({ content: t('更新成功') });
        setEditing(null);
        await loadInvoices();
      } else {
        Toast.error({ content: res.data?.message || t('更新失败') });
      }
    } catch (error) {
      Toast.error({ content: t('更新失败') });
    } finally {
      setSaving(false);
    }
  };

  const renderStatus = (value) => {
    const option = STATUS_OPTIONS.find((item) => item.value === value);
    return (
      <Tag color={option?.color || 'grey'} shape='circle'>
        {t(option?.label || value || '-')}
      </Tag>
    );
  };

  const columns = [
    ...(!adminView
      ? [
          {
            title: t('操作'),
            key: 'action',
            render: (_, record) =>
              record.status === 'payment_pending' ? (
                <Button
                  size='small'
                  theme='outline'
                  onClick={() => cancelInvoicePayment(record)}
                >
                  {t('取消待支付')}
                </Button>
              ) : (
                '-'
              ),
          },
        ]
      : adminView
        ? [
            {
              title: t('用户ID'),
              dataIndex: 'user_id',
              key: 'user_id',
              width: 90,
            },
          ]
        : []),
    {
      title: t('来源'),
      dataIndex: 'source_type',
      key: 'source_type',
      render: (value) => t(SOURCE_LABEL[value] || value || '-'),
    },
    {
      title: t('订单号'),
      dataIndex: 'source_id',
      key: 'source_id',
      render: (value) => <Text copyable>{value}</Text>,
    },
    {
      title: t('发票抬头类型'),
      dataIndex: 'invoice_type',
      key: 'invoice_type',
      render: (value) => t(TYPE_LABEL[value] || value || '-'),
    },
    {
      title: t('开票票种'),
      dataIndex: 'invoice_kind',
      key: 'invoice_kind',
      render: (value) => t(KIND_LABEL[value] || value || KIND_LABEL.normal),
    },
    {
      title: t('发票抬头'),
      dataIndex: 'title',
      key: 'title',
      render: (value, record) => (
        <div>
          <div>{value || '-'}</div>
          {record.tax_no && (
            <Text type='tertiary' size='small'>
              {record.tax_no}
            </Text>
          )}
        </div>
      ),
    },
    {
      title: t('费用'),
      dataIndex: 'total_amount',
      key: 'total_amount',
      render: (value, record) => (
        <div>
          <Text>¥{Number(value || 0).toFixed(2)}</Text>
          <br />
          <Text type='tertiary' size='small'>
            {t('含基础金额')} ¥{Number(record.base_amount || 0).toFixed(2)}
          </Text>
        </div>
      ),
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      key: 'status',
      render: renderStatus,
    },
    {
      title: t('发票文件'),
      dataIndex: 'download_url',
      key: 'download_url',
      render: (value) =>
        value ? (
          <Button
            size='small'
            theme='outline'
            onClick={() => window.open(value, '_blank')}
          >
            {t('下载')}
          </Button>
        ) : (
          '-'
        ),
    },
    {
      title: t('创建时间'),
      dataIndex: 'create_time',
      key: 'create_time',
      render: (value) => timestamp2string(value),
    },
    ...(adminView
      ? [
          {
            title: t('操作'),
            key: 'action',
            fixed: 'right',
            render: (_, record) => (
              <Button
                size='small'
                theme='outline'
                onClick={() => openEdit(record)}
              >
                {t('处理')}
              </Button>
            ),
          },
        ]
      : []),
  ];

  return (
    <div className='w-full max-w-7xl mx-auto relative min-h-screen lg:min-h-0 mt-[60px] px-2'>
      <Card
        title={adminView ? t('发票管理') : t('发票中心')}
        bodyStyle={{ padding: 16 }}
      >
        {adminView && (
          <Space style={{ marginBottom: 16 }}>
            <Select
              value={status}
              style={{ width: 160 }}
              placeholder={t('全部状态')}
              onChange={(value) => {
                setStatus(value || '');
                setPage(1);
              }}
              showClear
            >
              {STATUS_OPTIONS.map((item) => (
                <Select.Option key={item.value} value={item.value}>
                  {t(item.label)}
                </Select.Option>
              ))}
            </Select>
            <Button onClick={loadInvoices}>{t('刷新')}</Button>
          </Space>
        )}
        {!adminView && (
          <div className='mb-4 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
            <Text type='tertiary'>
              {t('可选择近 30 天内未开过发票的支付订单合并申请。')}
            </Text>
            <Button
              type='primary'
              theme='solid'
              onClick={() => setRequestVisible(true)}
            >
              {t('申请开票')}
            </Button>
          </div>
        )}
        <Table
          columns={columns}
          dataSource={records}
          rowKey='id'
          loading={loading}
          size='small'
          scroll={{ x: 'max-content' }}
          pagination={{
            currentPage: page,
            pageSize,
            total,
            showSizeChanger: true,
            pageSizeOpts: [10, 20, 50, 100],
            onPageChange: setPage,
            onPageSizeChange: (size) => {
              setPageSize(size);
              setPage(1);
            },
          }}
        />
      </Card>

      <Modal
        title={t('处理发票')}
        visible={!!editing}
        onCancel={() => setEditing(null)}
        onOk={saveInvoice}
        confirmLoading={saving}
      >
        <Form
          key={editing?.id || 'invoice-edit'}
          initValues={formValues}
          onValueChange={setFormValues}
        >
          <Form.Select
            field='status'
            label={t('状态')}
            style={{ width: '100%' }}
          >
            {(editing?.status === 'payment_pending'
              ? EDIT_STATUS_OPTIONS.filter((item) => item.value === 'closed')
              : EDIT_STATUS_OPTIONS
            ).map((item) => (
              <Form.Select.Option key={item.value} value={item.value}>
                {t(item.label)}
              </Form.Select.Option>
            ))}
          </Form.Select>
          <Form.Input
            field='download_url'
            label={t('发票下载 URL')}
            placeholder='https://example.com/invoice.pdf'
          />
          <Form.TextArea
            field='admin_remark'
            label={t('管理员备注')}
            autosize
          />
          {editing && (
            <TextArea
              disabled
              autosize
              value={`${t('接收邮箱')}: ${editing.email || '-'}\n${t('联系电话')}: ${
                editing.phone || '-'
              }\n${t('发票备注')}: ${editing.remark || '-'}`}
            />
          )}
        </Form>
      </Modal>

      {!adminView && (
        <InvoiceBatchRequestModal
          visible={requestVisible}
          onCancel={() => setRequestVisible(false)}
          onSuccess={loadInvoices}
          t={t}
        />
      )}
    </div>
  );
};

export default InvoiceCenter;
