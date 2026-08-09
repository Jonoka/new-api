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
import { useTranslation } from 'react-i18next';
import {
  API,
  renderQuota,
  showError,
  showSuccess,
  timestamp2string,
} from '../../helpers';
import {
  Button,
  Card,
  Col,
  Empty,
  Form,
  InputNumber,
  Modal,
  Row,
  Space,
  Spin,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { ArrowDownUp, Coins, Trophy } from 'lucide-react';

const { Text, Title } = Typography;

const readItems = (payload) => payload?.data?.items || [];

const statusColor = (status) => {
  switch (status) {
    case 'open':
      return 'green';
    case 'settled':
      return 'grey';
    case 'answered':
      return 'blue';
    case 'settling':
      return 'orange';
    default:
      return 'light-blue';
  }
};

const GameCenter = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [wallet, setWallet] = useState(null);
  const [predictions, setPredictions] = useState([]);
  const [transactions, setTransactions] = useState([]);
  const [quotaAmount, setQuotaAmount] = useState(1000);
  const [tokenAmount, setTokenAmount] = useState(1000);
  const [betVisible, setBetVisible] = useState(false);
  const [betPrediction, setBetPrediction] = useState(null);
  const [betOptionId, setBetOptionId] = useState(0);
  const [betAmount, setBetAmount] = useState(100);

  const loadData = async () => {
    setLoading(true);
    try {
      const [walletRes, predictionsRes, transactionsRes] = await Promise.all([
        API.get('/api/game/wallet'),
        API.get('/api/game/predictions'),
        API.get('/api/game/transactions'),
      ]);

      if (walletRes?.data?.success) {
        setWallet(walletRes.data.data);
      } else {
        showError(walletRes?.data?.message || t('游戏钱包加载失败'));
      }

      if (predictionsRes?.data?.success) {
        setPredictions(readItems(predictionsRes.data));
      } else {
        showError(predictionsRes?.data?.message || t('预测列表加载失败'));
      }

      if (transactionsRes?.data?.success) {
        setTransactions(readItems(transactionsRes.data));
      } else {
        setTransactions([]);
      }
    } catch (error) {
      showError(error.message || t('加载失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, []);

  const exchangeQuotaToToken = async () => {
    if (!quotaAmount || quotaAmount <= 0) {
      showError(t('请输入有效数量'));
      return;
    }
    const res = await API.post('/api/game/exchange/quota-to-token', {
      quota: Math.trunc(quotaAmount),
    });
    if (res?.data?.success) {
      showSuccess(t('兑换成功'));
      await loadData();
    } else {
      showError(res?.data?.message || t('兑换失败'));
    }
  };

  const exchangeTokenToQuota = async () => {
    if (!tokenAmount || tokenAmount <= 0) {
      showError(t('请输入有效数量'));
      return;
    }
    const res = await API.post('/api/game/exchange/token-to-quota', {
      tokens: Math.trunc(tokenAmount),
    });
    if (res?.data?.success) {
      showSuccess(t('兑换成功'));
      await loadData();
    } else {
      showError(res?.data?.message || t('兑换失败'));
    }
  };

  const openBet = (prediction, optionId) => {
    setBetPrediction(prediction);
    setBetOptionId(optionId);
    setBetAmount(100);
    setBetVisible(true);
  };

  const submitBet = async () => {
    if (!betPrediction || !betOptionId || !betAmount || betAmount <= 0) {
      showError(t('请输入有效数量'));
      return;
    }
    const res = await API.post(
      `/api/game/predictions/${betPrediction.id}/bets`,
      {
        option_id: betOptionId,
        amount: Math.trunc(betAmount),
      },
    );
    if (res?.data?.success) {
      showSuccess(t('下注成功'));
      setBetVisible(false);
      await loadData();
    } else {
      showError(res?.data?.message || t('下注失败'));
    }
  };

  const predictionColumns = useMemo(
    () => [
      {
        title: t('问题'),
        dataIndex: 'title',
        render: (text, record) => (
          <div className='max-w-md'>
            <div className='font-medium'>{text}</div>
            {record.description ? (
              <Text type='secondary' size='small'>
                {record.description}
              </Text>
            ) : null}
          </div>
        ),
      },
      {
        title: t('奖池'),
        dataIndex: 'total_pool',
      },
      {
        title: t('状态'),
        dataIndex: 'status',
        render: (status) => (
          <Tag color={statusColor(status)} shape='circle'>
            {t(status)}
          </Tag>
        ),
      },
      {
        title: t('截止时间'),
        dataIndex: 'close_time',
        render: (time) => (time ? timestamp2string(time) : '-'),
      },
      {
        title: t('选项'),
        dataIndex: 'options',
        render: (options = [], record) => (
          <Space wrap>
            {options.map((option) => (
              <Button
                key={option.id}
                size='small'
                type='tertiary'
                disabled={record.status !== 'open'}
                onClick={() => openBet(record, option.id)}
              >
                {option.title} ({option.pool_amount})
              </Button>
            ))}
          </Space>
        ),
      },
    ],
    [t],
  );

  const transactionColumns = useMemo(
    () => [
      {
        title: t('时间'),
        dataIndex: 'created_at',
        render: (time) => timestamp2string(time),
      },
      {
        title: t('类型'),
        dataIndex: 'type',
        render: (type) => t(type),
      },
      {
        title: t('Token 数量'),
        dataIndex: 'token_amount',
      },
      {
        title: t('手续费'),
        dataIndex: 'fee_amount',
      },
      {
        title: t('变动后余额'),
        dataIndex: 'balance_after',
      },
      {
        title: t('内容'),
        dataIndex: 'content',
      },
    ],
    [t],
  );

  return (
    <div className='mt-[60px] px-2'>
      <Spin spinning={loading}>
        <Space vertical align='start' style={{ width: '100%' }}>
          <div className='flex items-center justify-between w-full'>
            <div>
              <Title heading={3} className='m-0'>
                {t('游戏中心')}
              </Title>
              <Text type='secondary'>{t('游戏钱包、预测局和参与入口')}</Text>
            </div>
            <Button onClick={loadData}>{t('刷新')}</Button>
          </div>

          <Row gutter={[16, 16]} style={{ width: '100%' }}>
            <Col xs={24} lg={8}>
              <Card className='!rounded-2xl shadow-sm border-0'>
                <Space vertical align='start' style={{ width: '100%' }}>
                  <Space>
                    <Coins size={18} />
                    <Text strong>{t('游戏 Token 钱包')}</Text>
                  </Space>
                  <div className='text-3xl font-semibold tabular-nums'>
                    {wallet?.balance ?? 0}
                  </div>
                  <Text type='secondary'>
                    {t('游戏 Token 与账户额度相互独立')}
                  </Text>
                </Space>
              </Card>
            </Col>
            <Col xs={24} lg={16}>
              <Card className='!rounded-2xl shadow-sm border-0'>
                <Row gutter={[12, 12]}>
                  <Col xs={24} md={12}>
                    <Form.Label>{t('消耗额度兑换 Token')}</Form.Label>
                    <Space>
                      <InputNumber
                        min={1}
                        value={quotaAmount}
                        onChange={(value) => setQuotaAmount(value || 0)}
                      />
                      <Button
                        icon={<ArrowDownUp size={14} />}
                        onClick={exchangeQuotaToToken}
                      >
                        {t('兑换')}
                      </Button>
                    </Space>
                    <div className='mt-1 text-xs text-gray-500'>
                      {t('当前输入额度')}：{renderQuota(quotaAmount)}
                    </div>
                  </Col>
                  <Col xs={24} md={12}>
                    <Form.Label>{t('Token 兑换回额度')}</Form.Label>
                    <Space>
                      <InputNumber
                        min={1}
                        value={tokenAmount}
                        onChange={(value) => setTokenAmount(value || 0)}
                      />
                      <Button
                        type='tertiary'
                        icon={<ArrowDownUp size={14} />}
                        onClick={exchangeTokenToQuota}
                      >
                        {t('兑换')}
                      </Button>
                    </Space>
                  </Col>
                </Row>
              </Card>
            </Col>
          </Row>

          <Card
            className='!rounded-2xl shadow-sm border-0 w-full'
            title={
              <Space>
                <Trophy size={18} />
                {t('预测游戏')}
              </Space>
            }
          >
            <Table
              rowKey='id'
              columns={predictionColumns}
              dataSource={predictions}
              pagination={false}
              scroll={{ x: 760 }}
              empty={<Empty description={t('暂无预测')} />}
            />
          </Card>

          <Card
            className='!rounded-2xl shadow-sm border-0 w-full'
            title={t('游戏钱包流水')}
          >
            <Table
              rowKey='id'
              columns={transactionColumns}
              dataSource={transactions}
              pagination={false}
              scroll={{ x: 820 }}
              empty={<Empty description={t('暂无钱包流水')} />}
            />
          </Card>
        </Space>
      </Spin>

      <Modal
        title={t('预测下注')}
        visible={betVisible}
        onOk={submitBet}
        onCancel={() => setBetVisible(false)}
        maskClosable={false}
      >
        <Space vertical align='start' style={{ width: '100%' }}>
          <Text>{betPrediction?.title}</Text>
          <Form.Label>{t('下注 Token 数量')}</Form.Label>
          <InputNumber
            min={1}
            value={betAmount}
            onChange={(value) => setBetAmount(value || 0)}
            style={{ width: '100%' }}
          />
        </Space>
      </Modal>
    </div>
  );
};

export default GameCenter;
