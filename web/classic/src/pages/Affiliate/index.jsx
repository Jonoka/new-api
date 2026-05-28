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

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Avatar,
  Button,
  Card,
  Col,
  Empty,
  Input,
  InputNumber,
  Modal,
  Row,
  Select,
  Space,
  Spin,
  Table,
  Tag,
  TextArea,
  Typography,
} from '@douyinfe/semi-ui';
import {
  Banknote,
  Copy,
  HandCoins,
  Link2,
  RefreshCw,
  Send,
  Trophy,
  Upload,
  WalletCards,
} from 'lucide-react';
import {
  API,
  copy,
  renderQuota,
  showError,
  showSuccess,
  timestamp2string,
} from '../../helpers';
import { displayAmountToQuota, quotaToDisplayAmount } from '../../helpers/quota';

const { Text, Title } = Typography;

const EMPTY_ACCOUNT = {
  user_id: 0,
  usdt_address: '',
  usdt_chain: 'TRC20',
  alipay_account: '',
  alipay_name: '',
  alipay_qr_path: '',
  wechat_account: '',
  wechat_name: '',
  wechat_qr_path: '',
};

function getItems(pageData) {
  return pageData?.items || pageData?.Items || [];
}

function statusText(t, status) {
  const map = {
    pending: t('待到账'),
    available: t('可提现'),
    approved: t('已通过'),
    paid: t('已打款'),
    rejected: t('已驳回'),
  };
  return map[status] || status;
}

function statusColor(status) {
  if (status === 'paid' || status === 'available') return 'green';
  if (status === 'rejected') return 'red';
  if (status === 'approved') return 'blue';
  return 'orange';
}

function methodText(t, method) {
  if (method === 'usdt') return 'USDT';
  if (method === 'alipay') return t('支付宝');
  if (method === 'wechat') return t('微信');
  return method;
}

const Affiliate = () => {
  const { t } = useTranslation();
  const [summary, setSummary] = useState(null);
  const [account, setAccount] = useState(EMPTY_ACCOUNT);
  const [records, setRecords] = useState([]);
  const [withdrawals, setWithdrawals] = useState([]);
  const [leaderboard, setLeaderboard] = useState([]);
  const [leaderboardPeriod, setLeaderboardPeriod] = useState('month');
  const [loading, setLoading] = useState(false);
  const [savingAccount, setSavingAccount] = useState(false);
  const [uploadingMethod, setUploadingMethod] = useState('');
  const [withdrawVisible, setWithdrawVisible] = useState(false);
  const [withdrawMethod, setWithdrawMethod] = useState('alipay');
  const [withdrawAmount, setWithdrawAmount] = useState(0);
  const [withdrawLoading, setWithdrawLoading] = useState(false);
  const [transferLoading, setTransferLoading] = useState(false);

  const balance = summary?.balance || {};
  const availableAmount = useMemo(
    () => quotaToDisplayAmount(balance.available_quota || 0),
    [balance.available_quota],
  );

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const [
        summaryRes,
        accountRes,
        recordsRes,
        withdrawalsRes,
        leaderboardRes,
      ] = await Promise.all([
        API.get('/api/affiliate/summary'),
        API.get('/api/affiliate/payout-account'),
        API.get('/api/affiliate/records', {
          params: { p: 1, page_size: 20 },
        }),
        API.get('/api/affiliate/withdrawals', {
          params: { p: 1, page_size: 20 },
        }),
        API.get('/api/affiliate/leaderboard', {
          params: { period: leaderboardPeriod, limit: 20 },
        }),
      ]);

      if (summaryRes.data.success) setSummary(summaryRes.data.data);
      if (accountRes.data.success) setAccount(accountRes.data.data);
      if (recordsRes.data.success) setRecords(getItems(recordsRes.data.data));
      if (withdrawalsRes.data.success) {
        setWithdrawals(getItems(withdrawalsRes.data.data));
      }
      if (leaderboardRes.data.success) {
        setLeaderboard(leaderboardRes.data.data || []);
      }
    } catch (error) {
      showError(t('获取返佣数据失败'));
    } finally {
      setLoading(false);
    }
  }, [leaderboardPeriod, t]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const copyText = async (text, message) => {
    const ok = await copy(text || '');
    if (ok) showSuccess(message);
  };

  const handleAccountChange = (key, value) => {
    setAccount((current) => ({ ...current, [key]: value }));
  };

  const saveAccount = async () => {
    setSavingAccount(true);
    try {
      const payload = {
        ...account,
        usdt_chain: summary?.setting?.usdt_chain || account.usdt_chain,
      };
      const res = await API.put('/api/affiliate/payout-account', payload);
      if (res.data.success) {
        setAccount(res.data.data);
        showSuccess(t('收款账户已保存'));
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(t('保存失败'));
    } finally {
      setSavingAccount(false);
    }
  };

  const uploadQr = async (method, file) => {
    if (!file) return;
    const form = new FormData();
    form.append('method', method);
    form.append('file', file);
    setUploadingMethod(method);
    try {
      const res = await API.post('/api/affiliate/upload-qr', form, {
        headers: { 'Content-Type': 'multipart/form-data' },
      });
      if (res.data.success) {
        const pathKey =
          method === 'alipay' ? 'alipay_qr_path' : 'wechat_qr_path';
        setAccount((current) => ({ ...current, [pathKey]: res.data.data.path }));
        showSuccess(t('收款码已上传'));
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(t('上传失败'));
    } finally {
      setUploadingMethod('');
    }
  };

  const submitWithdraw = async () => {
    const quota = displayAmountToQuota(withdrawAmount);
    if (!Number.isFinite(quota) || quota <= 0) {
      showError(t('请输入有效提现金额'));
      return;
    }
    if (quota > (balance.available_quota || 0)) {
      showError(t('可提现额度不足'));
      return;
    }
    setWithdrawLoading(true);
    try {
      const res = await API.post('/api/affiliate/withdraw', {
        method: withdrawMethod,
        quota,
      });
      if (res.data.success) {
        showSuccess(t('提现申请已提交'));
        setWithdrawVisible(false);
        setWithdrawAmount(0);
        await refresh();
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(t('提交失败'));
    } finally {
      setWithdrawLoading(false);
    }
  };

  const transferAllToBalance = async () => {
    const quota = balance.available_quota || 0;
    if (quota <= 0) return;
    setTransferLoading(true);
    try {
      const res = await API.post('/api/affiliate/transfer-to-balance', {
        quota,
      });
      if (res.data.success) {
        showSuccess(t('已转入余额'));
        await refresh();
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(t('转入失败'));
    } finally {
      setTransferLoading(false);
    }
  };

  const recordColumns = [
    { title: t('层级'), dataIndex: 'level', width: 80 },
    {
      title: t('来源'),
      render: (_, record) => `${record.source_type} #${record.source_id}`,
    },
    {
      title: t('返佣额度'),
      dataIndex: 'reward_quota',
      render: (value) => renderQuota(value || 0),
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      render: (value) => (
        <Tag color={statusColor(value)}>{statusText(t, value)}</Tag>
      ),
    },
    {
      title: t('可提现时间'),
      dataIndex: 'available_time',
      render: (value) => (value ? timestamp2string(value) : '-'),
    },
  ];

  const withdrawalColumns = [
    {
      title: t('提现方式'),
      dataIndex: 'method',
      render: (value) => methodText(t, value),
    },
    {
      title: t('提现额度'),
      dataIndex: 'quota',
      render: (value) => renderQuota(value || 0),
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      render: (value) => (
        <Tag color={statusColor(value)}>{statusText(t, value)}</Tag>
      ),
    },
    {
      title: t('提交时间'),
      dataIndex: 'created_at',
      render: (value) => (value ? timestamp2string(value) : '-'),
    },
  ];

  const leaderboardColumns = [
    { title: t('排名'), dataIndex: 'rank', width: 80, render: (rank) => `#${rank}` },
    {
      title: t('用户'),
      render: (_, record) =>
        record.display_name || record.username || `ID ${record.user_id}`,
    },
    { title: t('邀请人数'), dataIndex: 'invite_count' },
    {
      title: t('返利金额'),
      dataIndex: 'commission_quota',
      render: (value) => renderQuota(value || 0),
    },
  ];

  return (
    <div className='w-full max-w-7xl mx-auto relative min-h-screen lg:min-h-0 mt-[60px] px-2'>
      <Spin spinning={loading}>
        <div className='flex flex-col gap-4'>
          <div className='flex flex-col gap-1'>
            <Title heading={3} style={{ margin: 0 }}>
              {t('返佣分成')}
            </Title>
            <Text type='secondary'>
              {t('查看邀请返佣、排行榜，并管理提现账户')}
            </Text>
          </div>

          <Row gutter={[16, 16]}>
            {[
              [t('待到账'), balance.pending_quota || 0],
              [t('可提现'), balance.available_quota || 0],
              [t('冻结中'), balance.frozen_quota || 0],
              [t('累计返佣'), balance.total_quota || 0],
            ].map(([label, value]) => (
              <Col xs={24} sm={12} lg={6} key={label}>
                <Card bodyStyle={{ padding: 18 }}>
                  <Text type='secondary'>{label}</Text>
                  <div className='mt-2 text-2xl font-semibold'>
                    {renderQuota(value)}
                  </div>
                </Card>
              </Col>
            ))}
          </Row>

          <Row gutter={[16, 16]}>
            <Col xs={24} lg={16}>
              <Card
                title={
                  <Space>
                    <Link2 size={16} />
                    {t('推广文案')}
                  </Space>
                }
              >
                <Space vertical align='start' style={{ width: '100%' }}>
                  <Input
                    value={summary?.invite_link || ''}
                    readonly
                    prefix={t('邀请链接')}
                    suffix={
                      <Button
                        icon={<Copy size={14} />}
                        onClick={() =>
                          copyText(summary?.invite_link, t('邀请链接已复制'))
                        }
                      >
                        {t('复制')}
                      </Button>
                    }
                  />
                  <TextArea
                    value={summary?.promotion_text || ''}
                    readonly
                    rows={4}
                    style={{ width: '100%' }}
                  />
                  <Button
                    icon={<Copy size={14} />}
                    onClick={() =>
                      copyText(summary?.promotion_text, t('推广文案已复制'))
                    }
                  >
                    {t('复制推广文案')}
                  </Button>
                  <Text type='secondary'>
                    {t('邀请人数')}：{summary?.aff_count || 0} ·{' '}
                    {t('一级返佣')}：{summary?.setting?.first_level_ratio || 0}% ·{' '}
                    {t('二级返佣')}：{summary?.setting?.second_level_ratio || 0}%
                  </Text>
                </Space>
              </Card>
            </Col>
            <Col xs={24} lg={8}>
              <Card
                title={
                  <Space>
                    <WalletCards size={16} />
                    {t('提现操作')}
                  </Space>
                }
              >
                <Space vertical align='start' style={{ width: '100%' }}>
                  <Text type='secondary'>{t('当前可提现')}</Text>
                  <div className='text-2xl font-semibold'>
                    {renderQuota(balance.available_quota || 0)}
                  </div>
                  <Button
                    type='primary'
                    icon={<Banknote size={14} />}
                    block
                    disabled={(balance.available_quota || 0) <= 0}
                    onClick={() => {
                      setWithdrawAmount(Number(availableAmount.toFixed(4)));
                      setWithdrawVisible(true);
                    }}
                  >
                    {t('申请提现')}
                  </Button>
                  <Button
                    icon={<Send size={14} />}
                    block
                    disabled={(balance.available_quota || 0) <= 0}
                    loading={transferLoading}
                    onClick={transferAllToBalance}
                  >
                    {t('转入余额')}
                  </Button>
                  <Button icon={<RefreshCw size={14} />} block onClick={refresh}>
                    {t('刷新')}
                  </Button>
                </Space>
              </Card>
            </Col>
          </Row>

          <Card
            title={
              <Space>
                <HandCoins size={16} />
                {t('收款账户')}
              </Space>
            }
          >
            <Row gutter={[16, 16]}>
              <Col xs={24} lg={8}>
                <Space vertical align='start' style={{ width: '100%' }}>
                  <Text strong>{t('USDT 地址')}</Text>
                  <Input
                    value={account.usdt_address || ''}
                    placeholder={t('请输入 USDT 地址')}
                    onChange={(value) =>
                      handleAccountChange('usdt_address', value)
                    }
                  />
                  <Text type='secondary'>
                    {t('当前提现链')}：{summary?.setting?.usdt_chain || 'TRC20'}
                  </Text>
                </Space>
              </Col>
              <Col xs={24} lg={8}>
                <Space vertical align='start' style={{ width: '100%' }}>
                  <Text strong>{t('支付宝收款')}</Text>
                  <Input
                    value={account.alipay_account || ''}
                    placeholder={t('账号或手机号')}
                    onChange={(value) =>
                      handleAccountChange('alipay_account', value)
                    }
                  />
                  <Input
                    value={account.alipay_name || ''}
                    placeholder={t('收款人姓名')}
                    onChange={(value) => handleAccountChange('alipay_name', value)}
                  />
                  <Button
                    icon={<Upload size={14} />}
                    loading={uploadingMethod === 'alipay'}
                    onClick={() =>
                      document.getElementById('affiliate-alipay-qr')?.click()
                    }
                  >
                    {account.alipay_qr_path ? t('重新上传收款码') : t('上传收款码')}
                  </Button>
                  <input
                    id='affiliate-alipay-qr'
                    type='file'
                    accept='image/*'
                    className='hidden'
                    onChange={(event) => uploadQr('alipay', event.target.files?.[0])}
                  />
                </Space>
              </Col>
              <Col xs={24} lg={8}>
                <Space vertical align='start' style={{ width: '100%' }}>
                  <Text strong>{t('微信收款')}</Text>
                  <Input
                    value={account.wechat_account || ''}
                    placeholder={t('账号或手机号')}
                    onChange={(value) =>
                      handleAccountChange('wechat_account', value)
                    }
                  />
                  <Input
                    value={account.wechat_name || ''}
                    placeholder={t('收款人姓名')}
                    onChange={(value) => handleAccountChange('wechat_name', value)}
                  />
                  <Button
                    icon={<Upload size={14} />}
                    loading={uploadingMethod === 'wechat'}
                    onClick={() =>
                      document.getElementById('affiliate-wechat-qr')?.click()
                    }
                  >
                    {account.wechat_qr_path ? t('重新上传收款码') : t('上传收款码')}
                  </Button>
                  <input
                    id='affiliate-wechat-qr'
                    type='file'
                    accept='image/*'
                    className='hidden'
                    onChange={(event) => uploadQr('wechat', event.target.files?.[0])}
                  />
                </Space>
              </Col>
            </Row>
            <Button
              type='primary'
              loading={savingAccount}
              onClick={saveAccount}
              style={{ marginTop: 16 }}
            >
              {t('保存收款账户')}
            </Button>
          </Card>

          <Card
            title={
              <div className='flex items-center justify-between gap-3'>
                <Space>
                  <Trophy size={16} />
                  {t('邀请排行榜')}
                </Space>
                <Select
                  value={leaderboardPeriod}
                  onChange={setLeaderboardPeriod}
                  style={{ width: 130 }}
                >
                  <Select.Option value='day'>{t('今日')}</Select.Option>
                  <Select.Option value='week'>{t('本周')}</Select.Option>
                  <Select.Option value='month'>{t('本月')}</Select.Option>
                </Select>
              </div>
            }
          >
            <Table
              rowKey='user_id'
              columns={leaderboardColumns}
              dataSource={leaderboard}
              pagination={false}
              size='small'
              empty={<Empty description={t('暂无排行榜数据')} />}
            />
          </Card>

          <Row gutter={[16, 16]}>
            <Col xs={24} xl={12}>
              <Card title={t('返佣明细')}>
                <Table
                  rowKey='id'
                  columns={recordColumns}
                  dataSource={records}
                  pagination={false}
                  size='small'
                  empty={<Empty description={t('暂无返佣记录')} />}
                />
              </Card>
            </Col>
            <Col xs={24} xl={12}>
              <Card title={t('提现记录')}>
                <Table
                  rowKey='id'
                  columns={withdrawalColumns}
                  dataSource={withdrawals}
                  pagination={false}
                  size='small'
                  empty={<Empty description={t('暂无提现记录')} />}
                />
              </Card>
            </Col>
          </Row>
        </div>
      </Spin>

      <Modal
        title={t('申请提现')}
        visible={withdrawVisible}
        onOk={submitWithdraw}
        onCancel={() => setWithdrawVisible(false)}
        confirmLoading={withdrawLoading}
        maskClosable={false}
        centered
      >
        <Space vertical align='start' style={{ width: '100%' }}>
          <Text type='secondary'>
            {t('提现申请提交后会冻结对应收益，管理员线下打款后标记完成。')}
          </Text>
          <Select
            value={withdrawMethod}
            onChange={setWithdrawMethod}
            style={{ width: '100%' }}
          >
            <Select.Option value='alipay'>{t('支付宝')}</Select.Option>
            <Select.Option value='wechat'>{t('微信')}</Select.Option>
            <Select.Option value='usdt'>USDT</Select.Option>
          </Select>
          <InputNumber
            value={withdrawAmount}
            min={0}
            precision={4}
            onChange={(value) => setWithdrawAmount(Number(value || 0))}
            style={{ width: '100%' }}
          />
          <Text type='secondary'>
            {t('当前可提现')}：{renderQuota(balance.available_quota || 0)}
          </Text>
        </Space>
      </Modal>
    </div>
  );
};

export default Affiliate;
