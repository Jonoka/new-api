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

import React, { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess, timestamp2string } from '../../helpers';
import {
  Button,
  Card,
  Empty,
  Form,
  Modal,
  SideSheet,
  Space,
  Spin,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { Plus, RefreshCw, Trophy } from 'lucide-react';

const { Text, Title } = Typography;

const readItems = (payload) => payload?.data?.items || [];

const toUnixSeconds = (date) => {
  if (!date) return 0;
  const ms = date instanceof Date ? date.getTime() : new Date(date).getTime();
  if (!Number.isFinite(ms)) return 0;
  return Math.floor(ms / 1000);
};

const statusColor = (status) => {
  switch (status) {
    case 'open':
      return 'green';
    case 'answered':
      return 'blue';
    case 'settling':
      return 'orange';
    case 'settled':
      return 'grey';
    default:
      return 'light-blue';
  }
};

const GameManagement = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [predictions, setPredictions] = useState([]);
  const [createVisible, setCreateVisible] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const formApiRef = useRef(null);

  const loadData = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/game/admin/predictions');
      if (res?.data?.success) {
        setPredictions(readItems(res.data));
      } else {
        showError(res?.data?.message || t('预测列表加载失败'));
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

  const handleCreate = async (values) => {
    const title = String(values.title || '').trim();
    const optionA = String(values.option_a || '').trim();
    const optionB = String(values.option_b || '').trim();
    const closeTime = toUnixSeconds(values.close_time);
    const settleTime = toUnixSeconds(values.settle_time);

    if (!title || !optionA || !optionB || !closeTime) {
      showError(t('请完整填写必填项'));
      return;
    }
    if (closeTime <= Math.floor(Date.now() / 1000)) {
      showError(t('截止时间必须晚于当前时间'));
      return;
    }
    if (settleTime > 0 && settleTime < closeTime) {
      showError(t('结算时间不能早于截止时间'));
      return;
    }

    setSubmitting(true);
    try {
      const res = await API.post('/api/game/admin/predictions', {
        title,
        description: String(values.description || '').trim(),
        options: [optionA, optionB],
        close_time: closeTime,
        settle_time: settleTime,
        judge_mode: values.judge_mode || 'manual',
      });
      if (res?.data?.success) {
        showSuccess(t('预测创建成功'));
        setCreateVisible(false);
        formApiRef.current?.reset();
        await loadData();
      } else {
        showError(res?.data?.message || t('创建失败'));
      }
    } catch (error) {
      showError(error.message || t('创建失败'));
    } finally {
      setSubmitting(false);
    }
  };

  const setAnswer = async (prediction, answerIndex) => {
    const res = await API.put(
      `/api/game/admin/predictions/${prediction.id}/answer`,
      { answer_index: answerIndex },
    );
    if (res?.data?.success) {
      showSuccess(t('答案已保存'));
      await loadData();
    } else {
      showError(res?.data?.message || t('保存失败'));
    }
  };

  const settlePrediction = async (prediction) => {
    Modal.confirm({
      title: t('确认结算该预测？'),
      content: t('结算后将按已设置答案派发奖励，此操作不可逆。'),
      onOk: async () => {
        const res = await API.post(
          `/api/game/admin/predictions/${prediction.id}/settle`,
        );
        if (res?.data?.success) {
          showSuccess(t('结算完成'));
          await loadData();
        } else {
          showError(res?.data?.message || t('结算失败'));
        }
      },
    });
  };

  const columns = useMemo(
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
        title: t('答案'),
        dataIndex: 'answer_option_id',
        render: (answerId, record) =>
          answerId
            ? record.options?.find((item) => item.id === answerId)?.title || '-'
            : '-',
      },
      {
        title: t('操作'),
        dataIndex: 'operate',
        fixed: 'right',
        render: (_, record) => (
          <Space wrap>
            {(record.options || []).map((option) => (
              <Button
                key={option.id}
                size='small'
                type='tertiary'
                onClick={() => setAnswer(record, option.index)}
                disabled={record.status === 'settled'}
              >
                {t('设为答案')} {option.index}
              </Button>
            ))}
            <Button
              size='small'
              type='primary'
              disabled={record.status === 'settled' || !record.answer_option_id}
              onClick={() => settlePrediction(record)}
            >
              {t('结算')}
            </Button>
          </Space>
        ),
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
                {t('游戏管理')}
              </Title>
              <Text type='secondary'>{t('预测局创建与结算')}</Text>
            </div>
            <Space>
              <Button icon={<RefreshCw size={14} />} onClick={loadData}>
                {t('刷新')}
              </Button>
              <Button
                type='primary'
                icon={<Plus size={14} />}
                onClick={() => setCreateVisible(true)}
              >
                {t('创建预测')}
              </Button>
            </Space>
          </div>

          <Card
            className='!rounded-2xl shadow-sm border-0 w-full'
            title={
              <Space>
                <Trophy size={18} />
                {t('预测局')}
              </Space>
            }
          >
            <Table
              rowKey='id'
              columns={columns}
              dataSource={predictions}
              pagination={false}
              scroll={{ x: 900 }}
              empty={<Empty description={t('暂无预测')} />}
            />
          </Card>
        </Space>
      </Spin>

      <SideSheet
        title={t('创建预测')}
        visible={createVisible}
        onCancel={() => setCreateVisible(false)}
        width={600}
        footer={
          <div className='flex justify-end gap-2'>
            <Button onClick={() => setCreateVisible(false)}>{t('取消')}</Button>
            <Button
              type='primary'
              loading={submitting}
              onClick={() => formApiRef.current?.submitForm()}
            >
              {t('保存')}
            </Button>
          </div>
        }
      >
        <Form
          getFormApi={(api) => (formApiRef.current = api)}
          initValues={{
            title: '',
            description: '',
            option_a: '',
            option_b: '',
            close_time: null,
            settle_time: null,
            judge_mode: 'manual',
          }}
          onSubmit={handleCreate}
        >
          <Form.Input
            field='title'
            label={t('问题')}
            placeholder={t('请输入预测问题')}
            rules={[{ required: true, message: t('请输入预测问题') }]}
          />
          <Form.TextArea
            field='description'
            label={t('描述')}
            placeholder={t('可选')}
            autosize
          />
          <Form.Input
            field='option_a'
            label={t('选项 1')}
            placeholder={t('请输入选项')}
            rules={[{ required: true, message: t('请输入选项') }]}
          />
          <Form.Input
            field='option_b'
            label={t('选项 2')}
            placeholder={t('请输入选项')}
            rules={[{ required: true, message: t('请输入选项') }]}
          />
          <Form.DatePicker
            field='close_time'
            label={t('截止时间')}
            type='dateTime'
            placeholder={t('选择截止时间')}
            style={{ width: '100%' }}
            rules={[{ required: true, message: t('请选择截止时间') }]}
          />
          <Form.DatePicker
            field='settle_time'
            label={t('结算时间')}
            type='dateTime'
            placeholder={t('可选')}
            style={{ width: '100%' }}
            showClear
          />
          <Form.Select
            field='judge_mode'
            label={t('判定方式')}
            style={{ width: '100%' }}
          >
            <Form.Select.Option value='manual'>
              {t('手动判定')}
            </Form.Select.Option>
            <Form.Select.Option value='auto'>
              {t('自动判定')}
            </Form.Select.Option>
          </Form.Select>
        </Form>
      </SideSheet>
    </div>
  );
};

export default GameManagement;
