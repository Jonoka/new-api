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
  Input,
  Modal,
  Select,
  Space,
  Spin,
  Switch,
  Tabs,
  Tag,
  TagInput,
  TextArea,
  Toast,
  Typography,
} from '@douyinfe/semi-ui';
import {
  BellRing,
  Bot,
  Pencil,
  Plus,
  Power,
  RefreshCw,
  Send,
  Trash2,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API, timestamp2string } from '../../helpers';

const { Text, Title } = Typography;

const FALLBACK_EVENT = {
  value: 'invoice_pending',
  label: '新的待开票订单',
  description: '有新的发票申请进入待开票状态时触发。',
  variables: [
    'mention',
    'invoice_id',
    'source_type',
    'source_id',
    'user_id',
    'title',
    'total_amount',
    'create_time',
  ],
  default_template:
    '{{mention}} 来新的发票订单啦~\n订单：{{invoice_id}}\n金额：{{total_amount}}',
};

const EMPTY_BOT = { name: '', token: '', enabled: true };
const EMPTY_TASK = {
  name: '',
  event_type: FALLBACK_EVENT.value,
  bot_id: 0,
  targets: [],
  template: FALLBACK_EVENT.default_template,
  enabled: true,
};

const normalizeList = (payload) => {
  if (Array.isArray(payload)) return payload;
  return payload?.items || [];
};

const formatTime = (value) => (value ? timestamp2string(value) : '-');

const getErrorMessage = (error, fallback) =>
  error?.response?.data?.message || error?.message || fallback;

const SectionEmpty = ({ icon, title, description }) => (
  <div className='flex min-h-52 flex-col items-center justify-center text-center'>
    {icon}
    <Text strong className='mt-3'>
      {title}
    </Text>
    <Text type='tertiary' size='small' className='mt-1'>
      {description}
    </Text>
  </div>
);

const NotificationCenter = () => {
  const { t } = useTranslation();
  const [tab, setTab] = useState('tasks');
  const [bots, setBots] = useState([]);
  const [tasks, setTasks] = useState([]);
  const [events, setEvents] = useState([]);
  const [deliveries, setDeliveries] = useState([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [testingId, setTestingId] = useState(null);
  const [botModalVisible, setBotModalVisible] = useState(false);
  const [taskModalVisible, setTaskModalVisible] = useState(false);
  const [editingBot, setEditingBot] = useState(null);
  const [editingTask, setEditingTask] = useState(null);
  const [botForm, setBotForm] = useState(EMPTY_BOT);
  const [taskForm, setTaskForm] = useState(EMPTY_TASK);
  const [taskChatIdInput, setTaskChatIdInput] = useState('');

  const eventOptions = events.length > 0 ? events : [FALLBACK_EVENT];
  const selectedEvent =
    eventOptions.find((item) => item.value === taskForm.event_type) ||
    FALLBACK_EVENT;

  const loadData = async () => {
    setLoading(true);
    try {
      const [botRes, taskRes, eventRes, deliveryRes] = await Promise.all([
        API.get('/api/notification/bots'),
        API.get('/api/notification/tasks'),
        API.get('/api/notification/event-types'),
        API.get('/api/notification/deliveries'),
      ]);
      const responses = [botRes, taskRes, eventRes, deliveryRes];
      const failed = responses.find(
        (response) => response.data?.success === false,
      );
      if (failed) throw new Error(failed.data?.message || t('加载失败'));
      setBots(normalizeList(botRes.data?.data));
      setTasks(normalizeList(taskRes.data?.data));
      setEvents(normalizeList(eventRes.data?.data));
      setDeliveries(normalizeList(deliveryRes.data?.data).slice(0, 5));
    } catch (error) {
      Toast.error({ content: getErrorMessage(error, t('加载失败')) });
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const openBotModal = (bot = null) => {
    setEditingBot(bot);
    setBotForm(
      bot
        ? { name: bot.name, token: '', enabled: bot.enabled }
        : { ...EMPTY_BOT },
    );
    setBotModalVisible(true);
  };

  const openTaskModal = (task = null) => {
    const defaultEvent = eventOptions[0] || FALLBACK_EVENT;
    setEditingTask(task);
    setTaskChatIdInput('');
    setTaskForm(
      task
        ? {
            name: task.name,
            event_type: task.event_type,
            bot_id: task.bot_id,
            targets: (task.targets || []).map((target) => ({ ...target })),
            template: task.template,
            enabled: task.enabled,
          }
        : {
            ...EMPTY_TASK,
            event_type: defaultEvent.value,
            bot_id: bots[0]?.id || 0,
            template: defaultEvent.default_template || '',
          },
    );
    setTaskModalVisible(true);
  };

  const saveBot = async () => {
    const name = botForm.name.trim();
    const token = botForm.token.trim();
    if (!name) {
      Toast.warning({ content: t('请输入机器人名称') });
      return;
    }
    if (!editingBot && !token) {
      Toast.warning({ content: t('请输入 Bot Token') });
      return;
    }
    setSaving(true);
    try {
      const payload = {
        name,
        enabled: botForm.enabled,
        ...(token ? { token } : {}),
      };
      const response = editingBot
        ? await API.put(`/api/notification/bots/${editingBot.id}`, payload)
        : await API.post('/api/notification/bots', payload);
      if (response.data?.success === false) {
        throw new Error(response.data?.message || t('保存失败'));
      }
      Toast.success({ content: t('Telegram Bot 已保存') });
      setBotModalVisible(false);
      await loadData();
    } catch (error) {
      Toast.error({ content: getErrorMessage(error, t('保存失败')) });
    } finally {
      setSaving(false);
    }
  };

  const saveTask = async () => {
    const pendingChatId = taskChatIdInput.trim();
    const sourceTargets =
      pendingChatId &&
      !taskForm.targets.some((target) => target.chat_id === pendingChatId)
        ? [
            ...taskForm.targets,
            {
              chat_id: pendingChatId,
              mention_user_id: '',
              mention_name: '',
              enabled: true,
            },
          ]
        : taskForm.targets;
    const targets = sourceTargets
      .map((target) => ({
        ...(target.id ? { id: target.id } : {}),
        chat_id: String(target.chat_id || '').trim(),
        ...(String(target.mention_user_id || '').trim()
          ? { mention_user_id: String(target.mention_user_id).trim() }
          : {}),
        ...(String(target.mention_name || '').trim()
          ? { mention_name: String(target.mention_name).trim() }
          : {}),
        enabled: target.enabled !== false,
      }))
      .filter((target) => target.chat_id);
    if (!taskForm.name.trim() || !taskForm.event_type || !taskForm.bot_id) {
      Toast.warning({ content: t('请填写完整的任务信息') });
      return;
    }
    if (targets.length === 0) {
      Toast.warning({ content: t('请至少添加一个接收 Chat ID') });
      return;
    }
    if (!taskForm.template.trim()) {
      Toast.warning({ content: t('请输入消息模板') });
      return;
    }
    setSaving(true);
    try {
      const payload = {
        ...taskForm,
        name: taskForm.name.trim(),
        template: taskForm.template.trim(),
        targets,
      };
      const response = editingTask
        ? await API.put(`/api/notification/tasks/${editingTask.id}`, payload)
        : await API.post('/api/notification/tasks', payload);
      if (response.data?.success === false) {
        throw new Error(response.data?.message || t('保存失败'));
      }
      Toast.success({ content: t('通知任务已保存') });
      setTaskChatIdInput('');
      setTaskModalVisible(false);
      await loadData();
    } catch (error) {
      Toast.error({ content: getErrorMessage(error, t('保存失败')) });
    } finally {
      setSaving(false);
    }
  };

  const disableItem = (type, item) => {
    Modal.confirm({
      title: type === 'bot' ? t('停用 Telegram Bot') : t('停用通知任务'),
      content: t('停用后将不再发送新的通知，确定继续吗？'),
      onOk: async () => {
        try {
          const response = await API.delete(
            `/api/notification/${type === 'bot' ? 'bots' : 'tasks'}/${item.id}`,
          );
          if (response.data?.success === false) {
            throw new Error(response.data?.message || t('操作失败'));
          }
          Toast.success({ content: t('停用成功') });
          await loadData();
        } catch (error) {
          Toast.error({ content: getErrorMessage(error, t('操作失败')) });
        }
      },
    });
  };

  const testBot = (bot) => {
    let chatId = '';
    Modal.confirm({
      title: t('测试 Telegram Bot'),
      content: (
        <div className='pt-2'>
          <Text type='tertiary' size='small'>
            {t('输入接收测试消息的 Chat ID。')}
          </Text>
          <Input
            className='mt-3'
            placeholder={t('Chat ID')}
            onChange={(value) => {
              chatId = value.trim();
            }}
          />
        </div>
      ),
      onOk: async () => {
        if (!chatId) {
          Toast.warning({ content: t('请输入 Chat ID') });
          return Promise.reject();
        }
        setTestingId(bot.id);
        try {
          const response = await API.post(
            `/api/notification/bots/${bot.id}/test`,
            { chat_id: chatId },
          );
          if (response.data?.success === false) {
            throw new Error(response.data?.message || t('测试失败'));
          }
          Toast.success({ content: t('测试消息已发送') });
          await loadData();
        } catch (error) {
          Toast.error({ content: getErrorMessage(error, t('测试失败')) });
          return Promise.reject(error);
        } finally {
          setTestingId(null);
        }
      },
    });
  };

  const updateChatIds = (chatIds) => {
    const normalized = [
      ...new Set(chatIds.map((id) => String(id).trim())),
    ].filter(Boolean);
    setTaskForm((current) => ({
      ...current,
      targets: normalized.map(
        (chatId) =>
          current.targets.find((target) => target.chat_id === chatId) || {
            chat_id: chatId,
            mention_user_id: '',
            mention_name: '',
            enabled: true,
          },
      ),
    }));
    setTaskChatIdInput('');
  };

  const updateTarget = (chatId, field, value) => {
    setTaskForm((current) => ({
      ...current,
      targets: current.targets.map((target) =>
        target.chat_id === chatId ? { ...target, [field]: value } : target,
      ),
    }));
  };

  const headerAction = useMemo(() => {
    if (tab === 'bots') {
      return (
        <Button icon={<Plus size={16} />} onClick={() => openBotModal()}>
          {t('添加 Telegram Bot')}
        </Button>
      );
    }
    if (tab === 'tasks') {
      return (
        <Button
          type='primary'
          icon={<Plus size={16} />}
          disabled={bots.length === 0}
          onClick={() => openTaskModal()}
        >
          {t('新建通知任务')}
        </Button>
      );
    }
    return null;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [bots.length, tab, t]);

  const renderTasks = () => {
    if (tasks.length === 0) {
      return (
        <SectionEmpty
          icon={<BellRing size={38} color='var(--semi-color-text-2)' />}
          title={t('暂无通知任务')}
          description={t(
            '创建任务后，可在支持的事件发生时自动发送 Telegram 消息。',
          )}
        />
      );
    }
    return (
      <div className='grid grid-cols-1 gap-3 lg:grid-cols-2'>
        {tasks.map((task) => (
          <Card
            key={task.id}
            title={
              <Space>
                <Text strong>{task.name}</Text>
                <Tag color={task.enabled ? 'green' : 'grey'}>
                  {task.enabled ? t('已启用') : t('已停用')}
                </Tag>
              </Space>
            }
            headerExtraContent={
              <Space spacing={4}>
                <Button
                  theme='borderless'
                  type='tertiary'
                  icon={<Pencil size={16} />}
                  aria-label={t('编辑')}
                  onClick={() => openTaskModal(task)}
                />
                <Button
                  theme='borderless'
                  type='danger'
                  icon={<Power size={16} />}
                  aria-label={t('停用')}
                  onClick={() => disableItem('task', task)}
                />
              </Space>
            }
          >
            <div className='grid grid-cols-2 gap-3 text-sm'>
              <div>
                <Text type='tertiary' size='small'>
                  {t('事件类型')}
                </Text>
                <div>{task.event_name || task.event_type}</div>
              </div>
              <div>
                <Text type='tertiary' size='small'>
                  Telegram Bot
                </Text>
                <div>{task.bot_name || `#${task.bot_id}`}</div>
              </div>
              <div>
                <Text type='tertiary' size='small'>
                  {t('接收人')}
                </Text>
                <div>{task.targets?.length || 0}</div>
              </div>
              <div>
                <Text type='tertiary' size='small'>
                  {t('最近触发')}
                </Text>
                <div>{formatTime(task.last_triggered_at)}</div>
              </div>
            </div>
          </Card>
        ))}
      </div>
    );
  };

  const renderBots = () => {
    if (bots.length === 0) {
      return (
        <SectionEmpty
          icon={<Bot size={38} color='var(--semi-color-text-2)' />}
          title={t('暂无 Telegram Bot')}
          description={t('请先添加并测试一个 Bot，再创建通知任务。')}
        />
      );
    }
    return (
      <div className='grid grid-cols-1 gap-3 lg:grid-cols-2'>
        {bots.map((bot) => (
          <Card
            key={bot.id}
            title={
              <Space>
                <Text strong>{bot.name}</Text>
                <Tag color={bot.enabled ? 'green' : 'grey'}>
                  {bot.enabled ? t('已启用') : t('已停用')}
                </Tag>
                {bot.token_configured && <Tag>{t('Token 已配置')}</Tag>}
              </Space>
            }
            headerExtraContent={
              <Space spacing={4}>
                <Button
                  theme='borderless'
                  icon={<Send size={16} />}
                  loading={testingId === bot.id}
                  aria-label={t('测试')}
                  onClick={() => testBot(bot)}
                />
                <Button
                  theme='borderless'
                  icon={<Pencil size={16} />}
                  aria-label={t('编辑')}
                  onClick={() => openBotModal(bot)}
                />
                <Button
                  theme='borderless'
                  type='danger'
                  icon={<Trash2 size={16} />}
                  aria-label={t('停用')}
                  onClick={() => disableItem('bot', bot)}
                />
              </Space>
            }
          >
            <div className='grid grid-cols-2 gap-3 text-sm'>
              <div>
                <Text type='tertiary' size='small'>
                  {t('机器人账号')}
                </Text>
                <div>{bot.username ? `@${bot.username}` : '-'}</div>
              </div>
              <div>
                <Text type='tertiary' size='small'>
                  {t('最近测试')}
                </Text>
                <div>{formatTime(bot.last_test_at)}</div>
              </div>
              <div className='col-span-2'>
                <Text type='tertiary' size='small'>
                  {t('测试结果')}
                </Text>
                <div className={bot.last_test_error ? 'text-red-500' : ''}>
                  {bot.last_test_error || (bot.last_test_at ? t('成功') : '-')}
                </div>
              </div>
            </div>
          </Card>
        ))}
      </div>
    );
  };

  const renderDeliveries = () => {
    if (deliveries.length === 0) {
      return (
        <SectionEmpty
          icon={<Send size={38} color='var(--semi-color-text-2)' />}
          title={t('暂无最近通知')}
          description={t('这里只保留并展示最近五条投递结果。')}
        />
      );
    }
    return (
      <div className='space-y-3'>
        {deliveries.map((delivery) => (
          <Card key={delivery.id}>
            <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
              <div>
                <Space>
                  <Text strong>
                    {delivery.task_name || `#${delivery.task_id}`}
                  </Text>
                  <Tag color={delivery.status === 'success' ? 'green' : 'red'}>
                    {delivery.status === 'success' ? t('成功') : t('失败')}
                  </Tag>
                </Space>
                <div className='mt-1'>
                  <Text type='tertiary' size='small'>
                    {delivery.event_type} · {formatTime(delivery.created_at)}
                  </Text>
                </div>
              </div>
              <div className='grid grid-cols-2 gap-x-8 gap-y-1 text-sm sm:text-right'>
                <Text type='tertiary'>{t('来源 ID')}</Text>
                <Text>{delivery.source_id || '-'}</Text>
                <Text type='tertiary'>{t('Chat ID')}</Text>
                <Text>{delivery.chat_id || '-'}</Text>
              </div>
            </div>
            {delivery.last_error && (
              <Banner
                className='mt-3'
                type='danger'
                description={delivery.last_error}
              />
            )}
          </Card>
        ))}
      </div>
    );
  };

  return (
    <div className='mx-auto mt-[60px] min-h-screen w-full max-w-7xl px-2 pb-8 lg:min-h-0'>
      <Card bodyStyle={{ padding: 16 }}>
        <div className='mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
          <div>
            <Title heading={4}>{t('通知中心')}</Title>
            <Text type='tertiary'>
              {t('统一管理通知任务与 Telegram Bot。')}
            </Text>
          </div>
          <Space>
            <Button
              theme='outline'
              icon={<RefreshCw size={16} />}
              loading={loading}
              onClick={loadData}
            >
              {t('刷新')}
            </Button>
            {headerAction}
          </Space>
        </div>

        <Spin spinning={loading}>
          <Tabs type='line' activeKey={tab} onChange={setTab}>
            <Tabs.TabPane tab={t('通知任务')} itemKey='tasks'>
              {bots.length === 0 && !loading && (
                <Banner
                  className='mb-4'
                  type='warning'
                  description={t(
                    '创建通知任务前，请先添加并测试 Telegram Bot。',
                  )}
                />
              )}
              {renderTasks()}
            </Tabs.TabPane>
            <Tabs.TabPane tab='Telegram Bot' itemKey='bots'>
              {renderBots()}
            </Tabs.TabPane>
            <Tabs.TabPane tab={t('最近通知')} itemKey='deliveries'>
              {renderDeliveries()}
            </Tabs.TabPane>
          </Tabs>
        </Spin>
      </Card>

      <Modal
        title={editingBot ? t('编辑 Telegram Bot') : t('添加 Telegram Bot')}
        visible={botModalVisible}
        confirmLoading={saving}
        onCancel={() => setBotModalVisible(false)}
        onOk={saveBot}
      >
        <Space
          vertical
          align='start'
          spacing='medium'
          style={{ width: '100%' }}
        >
          <div className='w-full'>
            <Text strong>{t('机器人名称')}</Text>
            <Input
              className='mt-2'
              value={botForm.name}
              placeholder={t('例如：发票通知机器人')}
              onChange={(name) =>
                setBotForm((current) => ({ ...current, name }))
              }
            />
          </div>
          <div className='w-full'>
            <Text strong>Bot Token</Text>
            <Input
              className='mt-2'
              mode='password'
              autoComplete='new-password'
              value={botForm.token}
              placeholder={
                editingBot
                  ? t('留空则保留原 Token')
                  : t('输入 BotFather 提供的 Token')
              }
              onChange={(token) =>
                setBotForm((current) => ({ ...current, token }))
              }
            />
            <div className='mt-1'>
              <Text type='tertiary' size='small'>
                {t('Token 仅保存在服务端，保存后不会再次回显。')}
              </Text>
            </div>
          </div>
          <div className='flex w-full items-center justify-between rounded border p-3'>
            <Text>{t('启用')}</Text>
            <Switch
              checked={botForm.enabled}
              onChange={(enabled) =>
                setBotForm((current) => ({ ...current, enabled }))
              }
            />
          </div>
        </Space>
      </Modal>

      <Modal
        title={editingTask ? t('编辑通知任务') : t('新建通知任务')}
        visible={taskModalVisible}
        width={720}
        confirmLoading={saving}
        onCancel={() => setTaskModalVisible(false)}
        onOk={saveTask}
      >
        <div className='max-h-[65vh] space-y-4 overflow-y-auto pr-2'>
          <div className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
            <div>
              <Text strong>{t('任务名称')}</Text>
              <Input
                className='mt-2'
                value={taskForm.name}
                placeholder={t('例如：待开票订单提醒')}
                onChange={(name) =>
                  setTaskForm((current) => ({ ...current, name }))
                }
              />
            </div>
            <div>
              <Text strong>Telegram Bot</Text>
              <Select
                className='mt-2'
                style={{ width: '100%' }}
                value={taskForm.bot_id || undefined}
                optionList={bots.map((bot) => ({
                  label: bot.name,
                  value: bot.id,
                }))}
                onChange={(bot_id) =>
                  setTaskForm((current) => ({ ...current, bot_id }))
                }
              />
            </div>
          </div>
          <div>
            <Text strong>{t('事件类型')}</Text>
            <Select
              className='mt-2'
              style={{ width: '100%' }}
              value={taskForm.event_type}
              optionList={eventOptions.map((event) => ({
                label: t(event.label),
                value: event.value,
              }))}
              onChange={(event_type) => {
                const nextEvent = eventOptions.find(
                  (event) => event.value === event_type,
                );
                setTaskForm((current) => ({
                  ...current,
                  event_type,
                  template:
                    current.template || nextEvent?.default_template || '',
                }));
              }}
            />
            {selectedEvent.description && (
              <div className='mt-1'>
                <Text type='tertiary' size='small'>
                  {t(selectedEvent.description)}
                </Text>
              </div>
            )}
          </div>
          <div>
            <Text strong>{t('接收 Chat ID')}</Text>
            <TagInput
              className='mt-2'
              style={{ width: '100%' }}
              value={taskForm.targets.map((target) => target.chat_id)}
              placeholder={t('输入 Chat ID 后按回车，可添加多个')}
              addOnBlur
              inputValue={taskChatIdInput}
              onChange={updateChatIds}
              onInputChange={(value) => setTaskChatIdInput(value)}
            />
            <div className='mt-1'>
              <Text type='tertiary' size='small'>
                {t('私聊需先主动联系 Bot；群组和频道 ID 可能为负数。')}
              </Text>
            </div>
          </div>
          {taskForm.targets.map((target) => (
            <div key={target.chat_id} className='rounded border p-3'>
              <Tag>{target.chat_id}</Tag>
              <div className='mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2'>
                <div>
                  <Text size='small'>{t('提及用户 ID')}</Text>
                  <Input
                    className='mt-1'
                    value={target.mention_user_id || ''}
                    placeholder={t('可选，Telegram 用户 ID')}
                    onChange={(value) =>
                      updateTarget(target.chat_id, 'mention_user_id', value)
                    }
                  />
                </div>
                <div>
                  <Text size='small'>{t('提及名称')}</Text>
                  <Input
                    className='mt-1'
                    value={target.mention_name || ''}
                    placeholder={t('可选，显示名称')}
                    onChange={(value) =>
                      updateTarget(target.chat_id, 'mention_name', value)
                    }
                  />
                </div>
              </div>
            </div>
          ))}
          <div>
            <Text strong>{t('消息模板')}</Text>
            <TextArea
              className='mt-2 font-mono'
              autosize={{ minRows: 6, maxRows: 12 }}
              value={taskForm.template}
              onChange={(template) =>
                setTaskForm((current) => ({ ...current, template }))
              }
            />
            <Space wrap className='mt-2'>
              {(selectedEvent.variables || []).map((variable) => (
                <Tag key={variable}>{`{{${variable}}}`}</Tag>
              ))}
            </Space>
          </div>
          <div className='flex items-center justify-between rounded border p-3'>
            <div>
              <Text>{t('启用')}</Text>
              <div>
                <Text type='tertiary' size='small'>
                  {t('只有启用的任务会接收新的事件。')}
                </Text>
              </div>
            </div>
            <Switch
              checked={taskForm.enabled}
              onChange={(enabled) =>
                setTaskForm((current) => ({ ...current, enabled }))
              }
            />
          </div>
        </div>
      </Modal>
    </div>
  );
};

export default NotificationCenter;
