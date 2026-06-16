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
  Collapse,
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
  Tabs,
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
  Trash2,
  Trophy,
  Upload,
  WalletCards,
} from 'lucide-react';
import {
  API,
  copy,
  renderQuota,
  renderQuotaWithAmount,
  showError,
  showSuccess,
  timestamp2string,
} from '../../helpers';
import {
  displayAmountToQuota,
  quotaToDisplayAmount,
} from '../../helpers/quota';
import { ITEMS_PER_PAGE } from '../../constants';

const { Text, Title } = Typography;

function AffiliateApplicationGate({
  t,
  reviewStatus,
  agreementConfirm,
  setAgreementConfirm,
  applyLoading,
  handleApply,
}) {
  const { status, eligibility, rejected_reason } = reviewStatus || {};

  // Pending — waiting for admin review
  if (status === 'pending') {
    return (
      <Card style={{ marginTop: 24 }}>
        <Space vertical align='center' style={{ width: '100%', padding: 40 }}>
          <Tag color='orange' size='large'>{t('审核中')}</Tag>
          <Text type='secondary'>
            {t('您的返佣参与申请正在审核中，请耐心等待管理员处理。')}
          </Text>
        </Space>
      </Card>
    );
  }

  // Rejected — show reason + allow re-apply
  if (status === 'rejected') {
    return (
      <Card style={{ marginTop: 24 }}>
        <Space vertical align='start' style={{ width: '100%', padding: 20 }}>
          <Tag color='red' size='large'>{t('申请被驳回')}</Tag>
          {rejected_reason && (
            <Text type='secondary'>
              {t('驳回原因')}：{rejected_reason}
            </Text>
          )}
          <Text type='secondary'>
            {t('您可以重新提交申请。')}
          </Text>
          <Button
            type='primary'
            loading={applyLoading}
            onClick={handleApply}
          >
            {t('重新申请')}
          </Button>
        </Space>
      </Card>
    );
  }

  // Not eligible
  if (eligibility && !eligibility.eligible) {
    return (
      <Card style={{ marginTop: 24 }}>
        <Space vertical align='start' style={{ width: '100%', padding: 20 }}>
          <Tag color='grey' size='large'>{t('暂不满足申请条件')}</Tag>
          {eligibility.reason && (
            <Text type='secondary'>{eligibility.reason}</Text>
          )}
        </Space>
      </Card>
    );
  }

  // Default: show agreement + apply form
  const agreementEnabled = reviewStatus?.agreement_enabled;
  const agreementText = reviewStatus?.agreement_text || '';
  const canSubmit = !agreementEnabled || agreementConfirm === '我已同意';

  return (
    <Card style={{ marginTop: 24 }}>
      <Space vertical align='start' style={{ width: '100%', padding: 20 }}>
        <Title heading={5}>{t('申请参与返佣')}</Title>
        <Text type='secondary'>
          {t('您需要提交申请并通过审核后才能参与返佣分成。')}
        </Text>
        {agreementEnabled && agreementText && (
          <>
            <Card
              style={{
                width: '100%',
                background: 'var(--semi-color-fill-0)',
                maxHeight: 300,
                overflow: 'auto',
              }}
            >
              <Text style={{ whiteSpace: 'pre-wrap' }}>{agreementText}</Text>
            </Card>
            <Input
              placeholder={t('请输入"我已同意"确认协议')}
              value={agreementConfirm}
              onChange={setAgreementConfirm}
              style={{ width: 260 }}
            />
          </>
        )}
        <Button
          type='primary'
          loading={applyLoading}
          disabled={!canSubmit}
          onClick={handleApply}
        >
          {t('提交申请')}
        </Button>
      </Space>
    </Card>
  );
}

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

const DEFAULT_PAYOUT_METHODS = ['usdt', 'alipay', 'wechat'];

function getItems(pageData) {
  if (Array.isArray(pageData)) return pageData;
  return pageData?.items || pageData?.Items || [];
}

function getTotal(pageData) {
  if (Array.isArray(pageData)) return pageData.length;
  return pageData?.total || pageData?.Total || 0;
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

function sourceFallbackText(record) {
  if (!record) return '-';
  return `${record.source_type} #${record.source_id}`;
}

function sourceDetailTitle(t, record) {
  const detail = record?.detail || {};
  if (record?.source_type === 'topup') return t('余额充值');
  if (record?.source_type === 'subscription') {
    return detail.plan_title
      ? `${t('订阅购买')}：${detail.plan_title}`
      : t('订阅购买');
  }
  if (record?.source_type === 'redemption') {
    return detail.redemption_name
      ? `${t('兑换码兑换')}：${detail.redemption_name}`
      : t('兑换码兑换');
  }
  if (detail.title) return detail.title;
  return sourceFallbackText(record);
}

function renderSourceDetail(t, record) {
  const detail = record?.detail || {};
  const parts = [];
  if (record.source_quota > 0) {
    parts.push(`${t('返佣基数')} ${renderQuota(record.source_quota)}`);
  }
  if (detail.original_amount > 0) {
    parts.push(`${t('原价')} ${renderQuotaWithAmount(detail.original_amount)}`);
  }
  if (detail.discount_amount > 0) {
    parts.push(
      `${t('已用优惠')} ${detail.promo_code || ''} -${renderQuotaWithAmount(
        detail.discount_amount,
      )}`.trim(),
    );
  }
  if (detail.paid_amount > 0 || detail.discount_amount > 0) {
    parts.push(
      `${t('实付')} ${renderQuotaWithAmount(detail.paid_amount || 0)}`,
    );
  }
  if (
    record.source_type === 'redemption' &&
    (detail.quota || record.source_quota)
  ) {
    parts.push(
      `${t('兑换额度')} ${renderQuota(detail.quota || record.source_quota)}`,
    );
  }
  if (detail.payment_provider || detail.payment_method) {
    parts.push(
      `${t('支付方式')} ${[detail.payment_provider, detail.payment_method]
        .filter(Boolean)
        .join('/')}`,
    );
  }
  return (
    <Space vertical align='start' spacing={2}>
      <Text strong>{sourceDetailTitle(t, record)}</Text>
      <Text type='secondary' size='small'>
        {parts.length > 0 ? parts.join(' · ') : sourceFallbackText(record)}
      </Text>
    </Space>
  );
}

function renderUserName(t, user, fallbackId) {
  if (!user && !fallbackId) return '-';
  const display =
    user?.display_name || user?.username || (fallbackId ? `ID ${fallbackId}` : '');
  return (
    <Space vertical align='start' spacing={2}>
      <Text strong>{display || '-'}</Text>
      {(user?.id || fallbackId) && (
        <Text type='secondary' size='small'>
          ID {user?.id || fallbackId}
        </Text>
      )}
    </Space>
  );
}

const Affiliate = () => {
  const { t } = useTranslation();
  const [summary, setSummary] = useState(null);
  const [account, setAccount] = useState(EMPTY_ACCOUNT);
  const [invitations, setInvitations] = useState([]);
  const [records, setRecords] = useState([]);
  const [withdrawals, setWithdrawals] = useState([]);
  const [inviteLeaderboard, setInviteLeaderboard] = useState([]);
  const [commissionLeaderboard, setCommissionLeaderboard] = useState([]);
  const [leaderboardPeriod, setLeaderboardPeriod] = useState('month');
  const [inviteLeaderboardPage, setInviteLeaderboardPage] = useState(1);
  const [inviteLeaderboardPageSize, setInviteLeaderboardPageSize] =
    useState(ITEMS_PER_PAGE);
  const [inviteLeaderboardTotal, setInviteLeaderboardTotal] = useState(0);
  const [commissionLeaderboardPage, setCommissionLeaderboardPage] = useState(1);
  const [commissionLeaderboardPageSize, setCommissionLeaderboardPageSize] =
    useState(ITEMS_PER_PAGE);
  const [commissionLeaderboardTotal, setCommissionLeaderboardTotal] =
    useState(0);
  const [invitationPage, setInvitationPage] = useState(1);
  const [invitationPageSize, setInvitationPageSize] = useState(ITEMS_PER_PAGE);
  const [invitationTotal, setInvitationTotal] = useState(0);
  const [recordPage, setRecordPage] = useState(1);
  const [recordPageSize, setRecordPageSize] = useState(ITEMS_PER_PAGE);
  const [recordTotal, setRecordTotal] = useState(0);
  const [withdrawalPage, setWithdrawalPage] = useState(1);
  const [withdrawalPageSize, setWithdrawalPageSize] = useState(ITEMS_PER_PAGE);
  const [withdrawalTotal, setWithdrawalTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [savingAccount, setSavingAccount] = useState(false);
  const [uploadingMethod, setUploadingMethod] = useState('');
  const [withdrawVisible, setWithdrawVisible] = useState(false);
  const [withdrawMethod, setWithdrawMethod] = useState('alipay');
  const [withdrawAmount, setWithdrawAmount] = useState(0);
  const [withdrawLoading, setWithdrawLoading] = useState(false);
  const [transferLoading, setTransferLoading] = useState(false);

  // Anti-fraud application state
  const [reviewStatus, setReviewStatus] = useState(null); // null = loading, object = loaded
  const [reviewLoading, setReviewLoading] = useState(true);
  const [applyLoading, setApplyLoading] = useState(false);
  const [agreementConfirm, setAgreementConfirm] = useState('');

  // Load application status on mount
  useEffect(() => {
    const checkApplicationStatus = async () => {
      setReviewLoading(true);
      try {
        const [statusRes, agreementRes] = await Promise.all([
          API.get('/api/affiliate/application-status'),
          API.get('/api/affiliate/agreement'),
        ]);
        if (statusRes.data.success) {
          const data = statusRes.data.data || {};
          // Merge agreement info from the agreement endpoint
          if (agreementRes.data.success) {
            const agr = agreementRes.data.data || {};
            data.agreement_enabled = agr.agreement_enabled;
            data.agreement_text = agr.agreement_text;
          }
          setReviewStatus(data);
        } else {
          setReviewStatus({ review_enabled: false });
        }
      } catch (error) {
        setReviewStatus({ review_enabled: false });
      } finally {
        setReviewLoading(false);
      }
    };
    checkApplicationStatus();
  }, []);

  const handleApply = async () => {
    setApplyLoading(true);
    try {
      const res = await API.post('/api/affiliate/apply', {
        agreement_accepted: true,
      });
      if (res.data.success) {
        showSuccess(t('申请已提交'));
        // Reload status
        const statusRes = await API.get('/api/affiliate/application-status');
        if (statusRes.data.success) {
          setReviewStatus(statusRes.data.data);
        }
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(t('提交失败'));
    } finally {
      setApplyLoading(false);
    }
  };

  const balance = summary?.balance || {};
  const availableAmount = useMemo(
    () => quotaToDisplayAmount(balance.available_quota || 0),
    [balance.available_quota],
  );
  const payoutMethods = useMemo(() => {
    const methods = summary?.setting?.payout_methods || [];
    return methods.length > 0 ? methods : DEFAULT_PAYOUT_METHODS;
  }, [summary?.setting?.payout_methods]);
  const isPayoutMethodEnabled = (method) => payoutMethods.includes(method);

  useEffect(() => {
    if (!payoutMethods.includes(withdrawMethod)) {
      setWithdrawMethod(payoutMethods[0] || 'usdt');
    }
  }, [payoutMethods, withdrawMethod]);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const [
        summaryRes,
        accountRes,
        invitationsRes,
        recordsRes,
        withdrawalsRes,
        inviteLeaderboardRes,
        commissionLeaderboardRes,
      ] = await Promise.all([
        API.get('/api/affiliate/summary'),
        API.get('/api/affiliate/payout-account'),
        API.get('/api/affiliate/invitations', {
          params: { p: invitationPage, page_size: invitationPageSize },
        }),
        API.get('/api/affiliate/records', {
          params: { p: recordPage, page_size: recordPageSize },
        }),
        API.get('/api/affiliate/withdrawals', {
          params: { p: withdrawalPage, page_size: withdrawalPageSize },
        }),
        API.get('/api/affiliate/leaderboard', {
          params: {
            period: leaderboardPeriod,
            sort: 'invites',
            metric: 'invites',
            p: inviteLeaderboardPage,
            page_size: inviteLeaderboardPageSize,
          },
        }),
        API.get('/api/affiliate/leaderboard', {
          params: {
            period: leaderboardPeriod,
            sort: 'commission',
            metric: 'commission',
            p: commissionLeaderboardPage,
            page_size: commissionLeaderboardPageSize,
          },
        }),
      ]);

      if (summaryRes.data.success) setSummary(summaryRes.data.data);
      if (accountRes.data.success) setAccount(accountRes.data.data);
      if (invitationsRes.data.success) {
        const pageData = invitationsRes.data.data;
        setInvitations(getItems(pageData));
        setInvitationPage(pageData?.page || invitationPage);
        setInvitationPageSize(pageData?.page_size || invitationPageSize);
        setInvitationTotal(getTotal(pageData));
      }
      if (recordsRes.data.success) {
        const pageData = recordsRes.data.data;
        setRecords(getItems(pageData));
        setRecordPage(pageData?.page || recordPage);
        setRecordPageSize(pageData?.page_size || recordPageSize);
        setRecordTotal(getTotal(pageData));
      }
      if (withdrawalsRes.data.success) {
        const pageData = withdrawalsRes.data.data;
        setWithdrawals(getItems(pageData));
        setWithdrawalPage(pageData?.page || withdrawalPage);
        setWithdrawalPageSize(pageData?.page_size || withdrawalPageSize);
        setWithdrawalTotal(getTotal(pageData));
      }
      if (inviteLeaderboardRes.data.success) {
        const pageData = inviteLeaderboardRes.data.data;
        setInviteLeaderboard(getItems(pageData));
        setInviteLeaderboardPage(pageData?.page || inviteLeaderboardPage);
        setInviteLeaderboardPageSize(
          pageData?.page_size || inviteLeaderboardPageSize,
        );
        setInviteLeaderboardTotal(getTotal(pageData));
      }
      if (commissionLeaderboardRes.data.success) {
        const pageData = commissionLeaderboardRes.data.data;
        setCommissionLeaderboard(getItems(pageData));
        setCommissionLeaderboardPage(
          pageData?.page || commissionLeaderboardPage,
        );
        setCommissionLeaderboardPageSize(
          pageData?.page_size || commissionLeaderboardPageSize,
        );
        setCommissionLeaderboardTotal(getTotal(pageData));
      }
    } catch (error) {
      showError(t('获取返佣数据失败'));
    } finally {
      setLoading(false);
    }
  }, [
    commissionLeaderboardPage,
    commissionLeaderboardPageSize,
    invitationPage,
    invitationPageSize,
    inviteLeaderboardPage,
    inviteLeaderboardPageSize,
    leaderboardPeriod,
    recordPage,
    recordPageSize,
    t,
    withdrawalPage,
    withdrawalPageSize,
  ]);

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
        setAccount(
          res.data.data?.account ||
            ((current) => ({ ...current, [pathKey]: res.data.data.path })),
        );
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

  const deleteQr = async (method) => {
    try {
      const res = await API.delete('/api/affiliate/qr', {
        params: { method },
      });
      if (res.data.success) {
        setAccount(res.data.data || EMPTY_ACCOUNT);
        showSuccess(t('收款码已删除'));
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(t('删除失败'));
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
      title: t('购买用户'),
      width: 160,
      render: (_, record) => renderUserName(t, record.invitee, record.invitee_id),
    },
    {
      title: t('购买明细'),
      render: (_, record) => renderSourceDetail(t, record),
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

  const invitationColumns = [
    {
      title: t('被邀请用户'),
      render: (_, record) => renderUserName(t, record.invitee),
    },
    {
      title: t('注册时间'),
      dataIndex: ['invitee', 'created_at'],
      render: (value) => (value ? timestamp2string(value) : '-'),
    },
    {
      title: t('充值次数'),
      dataIndex: 'topup_count',
      render: (value) => value || 0,
    },
    {
      title: t('充值金额'),
      dataIndex: 'topup_quota',
      render: (value) => renderQuota(value || 0),
    },
    {
      title: t('贡献返利'),
      dataIndex: 'commission_quota',
      render: (value) => renderQuota(value || 0),
    },
    {
      title: t('最近充值'),
      dataIndex: 'last_topup_time',
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

  const inviteLeaderboardColumns = [
    {
      title: t('排名'),
      dataIndex: 'rank',
      width: 80,
      render: (rank) => `#${rank}`,
    },
    {
      title: t('用户'),
      render: (_, record) =>
        record.masked_name || `ID ${record.user_id}`,
    },
    { title: t('邀请人数'), dataIndex: 'invite_count' },
  ];

  const commissionLeaderboardColumns = [
    {
      title: t('排名'),
      dataIndex: 'rank',
      width: 80,
      render: (rank) => `#${rank}`,
    },
    {
      title: t('用户'),
      render: (_, record) =>
        record.masked_name || `ID ${record.user_id}`,
    },
    {
      title: t('返利金额'),
      dataIndex: 'commission_quota',
      render: (value) => renderQuota(value || 0),
    },
  ];

  return (
    <div className='w-full max-w-7xl mx-auto relative min-h-screen lg:min-h-0 mt-[60px] px-2'>
      {reviewLoading ? (
        <Spin spinning={true}>
          <div style={{ minHeight: 200 }} />
        </Spin>
      ) : reviewStatus?.review_enabled && reviewStatus?.status !== 'approved' ? (
        <AffiliateApplicationGate
          t={t}
          reviewStatus={reviewStatus}
          agreementConfirm={agreementConfirm}
          setAgreementConfirm={setAgreementConfirm}
          applyLoading={applyLoading}
          handleApply={handleApply}
        />
      ) : (
      <>
      {/* Anti-fraud agreement notice for approved/existing users */}
      {reviewStatus?.agreement_enabled && reviewStatus?.agreement_text && (
        <Card
          style={{ marginBottom: 16, border: '1px solid var(--semi-color-warning)' }}
          bodyStyle={{ padding: '12px 20px' }}
        >
          <Space vertical align='start' style={{ width: '100%' }}>
            <Space>
              <Tag color='orange' size='small'>{t('反欺诈公告')}</Tag>
              <Text strong>{t('参与返佣须知')}</Text>
            </Space>
            <Text type='secondary' style={{ whiteSpace: 'pre-wrap', fontSize: 13 }}>
              {reviewStatus.agreement_text}
            </Text>
          </Space>
        </Card>
      )}
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

          <Card
            title={
              <div className='flex items-center justify-between gap-3'>
                <Space>
                  <WalletCards size={16} />
                  {t('概览')}
                </Space>
                <Button
                  icon={<RefreshCw size={14} />}
                  theme='borderless'
                  type='tertiary'
                  onClick={refresh}
                  aria-label={t('刷新')}
                />
              </div>
            }
          >
            <div className='grid gap-4 xl:grid-cols-[minmax(0,1fr)_280px] xl:items-end'>
              <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
                {[
                  [t('可提现'), balance.available_quota || 0],
                  [t('待到账'), balance.pending_quota || 0],
                  [t('冻结中'), balance.frozen_quota || 0],
                  [t('累计返佣'), balance.total_quota || 0],
                ].map(([label, value], index) => (
                  <div className='min-w-0' key={label}>
                    <Text type='secondary'>{label}</Text>
                    <div
                      className={
                        index === 0
                          ? 'mt-1 text-2xl font-semibold'
                          : 'mt-1 text-lg font-semibold'
                      }
                    >
                      {renderQuota(value)}
                    </div>
                  </div>
                ))}
              </div>
              <div className='grid gap-2 sm:grid-cols-2 xl:grid-cols-1'>
                <Button
                  type='primary'
                  icon={<Banknote size={14} />}
                  block
                  disabled={(balance.available_quota || 0) <= 0}
                  onClick={() => {
                    if (!payoutMethods.length) {
                      showError(t('暂无可用提现渠道'));
                      return;
                    }
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
              </div>
            </div>
          </Card>

          <Card
            title={
              <Space>
                <Link2 size={16} />
                {t('推广文案')}
              </Space>
            }
          >
            <Space vertical align='start' style={{ width: '100%' }}>
              <div className='grid gap-2 sm:grid-cols-[1fr_auto] w-full'>
                <Input
                  value={summary?.invite_link || ''}
                  readonly
                  prefix={t('邀请链接')}
                />
                <Button
                  icon={<Copy size={14} />}
                  onClick={() =>
                    copyText(summary?.invite_link, t('邀请链接已复制'))
                  }
                >
                  {t('复制')}
                </Button>
              </div>
              <TextArea
                value={summary?.promotion_text || ''}
                readonly
                rows={3}
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
                {t('邀请人数')}：{summary?.aff_count || 0} · {t('一级返佣')}：
                {summary?.setting?.first_level_ratio || 0}% · {t('二级返佣')}：
                {summary?.setting?.second_level_ratio || 0}%
              </Text>
            </Space>
          </Card>

          <Card
            title={
              <div className='flex items-center justify-between gap-3'>
                <Space>
                  <HandCoins size={16} />
                  {t('收款账户')}
                </Space>
                <Space wrap>
                  {payoutMethods.map((method) => (
                    <Tag key={method}>{methodText(t, method)}</Tag>
                  ))}
                </Space>
              </div>
            }
          >
            <Collapse>
              <Collapse.Panel
                header={t('编辑收款账户')}
                itemKey='payout-account'
              >
                <Row gutter={[16, 16]}>
                  {isPayoutMethodEnabled('usdt') && (
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
                          {t('当前提现链')}：
                          {summary?.setting?.usdt_chain || 'TRC20'}
                        </Text>
                      </Space>
                    </Col>
                  )}
                  {isPayoutMethodEnabled('alipay') && (
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
                          onChange={(value) =>
                            handleAccountChange('alipay_name', value)
                          }
                        />
                        <Button
                          icon={<Upload size={14} />}
                          loading={uploadingMethod === 'alipay'}
                          onClick={() =>
                            document
                              .getElementById('affiliate-alipay-qr')
                              ?.click()
                          }
                        >
                          {account.alipay_qr_path
                            ? t('重新上传收款码')
                            : t('上传收款码')}
                        </Button>
                        {account.alipay_qr_path && (
                          <Space>
                            <img
                              src={account.alipay_qr_path}
                              alt={t('支付宝收款码')}
                              style={{
                                width: 48,
                                height: 48,
                                objectFit: 'cover',
                                borderRadius: 6,
                                border: '1px solid var(--semi-color-border)',
                              }}
                            />
                            <Button
                              type='danger'
                              theme='borderless'
                              icon={<Trash2 size={14} />}
                              onClick={() => deleteQr('alipay')}
                            >
                              {t('删除收款码')}
                            </Button>
                          </Space>
                        )}
                        <input
                          id='affiliate-alipay-qr'
                          type='file'
                          accept='image/*'
                          className='hidden'
                          onChange={(event) => {
                            uploadQr('alipay', event.target.files?.[0]);
                            event.target.value = '';
                          }}
                        />
                      </Space>
                    </Col>
                  )}
                  {isPayoutMethodEnabled('wechat') && (
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
                          onChange={(value) =>
                            handleAccountChange('wechat_name', value)
                          }
                        />
                        <Button
                          icon={<Upload size={14} />}
                          loading={uploadingMethod === 'wechat'}
                          onClick={() =>
                            document
                              .getElementById('affiliate-wechat-qr')
                              ?.click()
                          }
                        >
                          {account.wechat_qr_path
                            ? t('重新上传收款码')
                            : t('上传收款码')}
                        </Button>
                        {account.wechat_qr_path && (
                          <Space>
                            <img
                              src={account.wechat_qr_path}
                              alt={t('微信收款码')}
                              style={{
                                width: 48,
                                height: 48,
                                objectFit: 'cover',
                                borderRadius: 6,
                                border: '1px solid var(--semi-color-border)',
                              }}
                            />
                            <Button
                              type='danger'
                              theme='borderless'
                              icon={<Trash2 size={14} />}
                              onClick={() => deleteQr('wechat')}
                            >
                              {t('删除收款码')}
                            </Button>
                          </Space>
                        )}
                        <input
                          id='affiliate-wechat-qr'
                          type='file'
                          accept='image/*'
                          className='hidden'
                          onChange={(event) => {
                            uploadQr('wechat', event.target.files?.[0]);
                            event.target.value = '';
                          }}
                        />
                      </Space>
                    </Col>
                  )}
                </Row>
                <Button
                  type='primary'
                  loading={savingAccount}
                  onClick={saveAccount}
                  style={{ marginTop: 16 }}
                >
                  {t('保存收款账户')}
                </Button>
              </Collapse.Panel>
            </Collapse>
          </Card>

          <Card
            title={
              <div className='flex items-center gap-3'>
                <Space>
                  <Trophy size={16} />
                  {t('返佣动态')}
                </Space>
              </div>
            }
          >
            <Tabs type='line' defaultActiveKey='leaderboard'>
              <Tabs.TabPane tab={t('邀请排行榜')} itemKey='leaderboard'>
                <Space wrap style={{ marginBottom: 12 }}>
                  <Select
                    value={leaderboardPeriod}
                    onChange={(value) => {
                      setLeaderboardPeriod(value);
                      setInviteLeaderboardPage(1);
                      setCommissionLeaderboardPage(1);
                    }}
                    style={{ width: 130 }}
                  >
                    <Select.Option value='day'>{t('今日')}</Select.Option>
                    <Select.Option value='week'>{t('本周')}</Select.Option>
                    <Select.Option value='month'>{t('本月')}</Select.Option>
                  </Select>
                </Space>
                <div className='grid gap-4 xl:grid-cols-2'>
                  <div>
                    <Text strong>{t('邀请人数榜')}</Text>
                    <Table
                      rowKey='user_id'
                      columns={inviteLeaderboardColumns}
                      dataSource={inviteLeaderboard}
                      pagination={{
                        currentPage: inviteLeaderboardPage,
                        pageSize: inviteLeaderboardPageSize,
                        total: inviteLeaderboardTotal,
                        pageSizeOpts: [10, 20, 50, 100],
                        showSizeChanger: true,
                        onPageChange: setInviteLeaderboardPage,
                        onPageSizeChange: (pageSize) => {
                          setInviteLeaderboardPageSize(pageSize);
                          setInviteLeaderboardPage(1);
                        },
                      }}
                      size='small'
                      scroll={{ x: 420 }}
                      empty={<Empty description={t('暂无邀请人数榜数据')} />}
                      style={{ marginTop: 8 }}
                    />
                  </div>
                  <div>
                    <Text strong>{t('返利金额榜')}</Text>
                    <Table
                      rowKey='user_id'
                      columns={commissionLeaderboardColumns}
                      dataSource={commissionLeaderboard}
                      pagination={{
                        currentPage: commissionLeaderboardPage,
                        pageSize: commissionLeaderboardPageSize,
                        total: commissionLeaderboardTotal,
                        pageSizeOpts: [10, 20, 50, 100],
                        showSizeChanger: true,
                        onPageChange: setCommissionLeaderboardPage,
                        onPageSizeChange: (pageSize) => {
                          setCommissionLeaderboardPageSize(pageSize);
                          setCommissionLeaderboardPage(1);
                        },
                      }}
                      size='small'
                      scroll={{ x: 420 }}
                      empty={<Empty description={t('暂无返利金额榜数据')} />}
                      style={{ marginTop: 8 }}
                    />
                  </div>
                </div>
              </Tabs.TabPane>
              <Tabs.TabPane tab={t('邀请动态')} itemKey='invitations'>
                <Table
                  rowKey={(record) => record.invitee?.id}
                  columns={invitationColumns}
                  dataSource={invitations}
                  pagination={{
                    currentPage: invitationPage,
                    pageSize: invitationPageSize,
                    total: invitationTotal,
                    pageSizeOpts: [10, 20, 50, 100],
                    showSizeChanger: true,
                    onPageChange: setInvitationPage,
                    onPageSizeChange: (pageSize) => {
                      setInvitationPageSize(pageSize);
                      setInvitationPage(1);
                    },
                  }}
                  size='small'
                  scroll={{ x: 840 }}
                  empty={<Empty description={t('暂无邀请动态')} />}
                />
              </Tabs.TabPane>
              <Tabs.TabPane tab={t('返佣明细')} itemKey='records'>
                <Table
                  rowKey='id'
                  columns={recordColumns}
                  dataSource={records}
                  pagination={{
                    currentPage: recordPage,
                    pageSize: recordPageSize,
                    total: recordTotal,
                    pageSizeOpts: [10, 20, 50, 100],
                    showSizeChanger: true,
                    onPageChange: setRecordPage,
                    onPageSizeChange: (pageSize) => {
                      setRecordPageSize(pageSize);
                      setRecordPage(1);
                    },
                  }}
                  size='small'
                  scroll={{ x: 820 }}
                  empty={<Empty description={t('暂无返佣记录')} />}
                />
              </Tabs.TabPane>
              <Tabs.TabPane tab={t('提现记录')} itemKey='withdrawals'>
                <Table
                  rowKey='id'
                  columns={withdrawalColumns}
                  dataSource={withdrawals}
                  pagination={{
                    currentPage: withdrawalPage,
                    pageSize: withdrawalPageSize,
                    total: withdrawalTotal,
                    pageSizeOpts: [10, 20, 50, 100],
                    showSizeChanger: true,
                    onPageChange: setWithdrawalPage,
                    onPageSizeChange: (pageSize) => {
                      setWithdrawalPageSize(pageSize);
                      setWithdrawalPage(1);
                    },
                  }}
                  size='small'
                  scroll={{ x: 560 }}
                  empty={<Empty description={t('暂无提现记录')} />}
                />
              </Tabs.TabPane>
            </Tabs>
          </Card>
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
            {payoutMethods.map((method) => (
              <Select.Option key={method} value={method}>
                {methodText(t, method)}
              </Select.Option>
            ))}
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
      </>
      )}
    </div>
  );
};

export default Affiliate;
