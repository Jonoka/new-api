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

import React, { useState, useEffect, useContext } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Card,
  Form,
  Button,
  Switch,
  Row,
  Col,
  Typography,
  Input,
  InputNumber,
  Select,
} from '@douyinfe/semi-ui';
import { IconDelete, IconPlus } from '@douyinfe/semi-icons';
import { API, showSuccess, showError } from '../../../helpers';
import { StatusContext } from '../../../context/Status';
import {
  DEFAULT_ADMIN_CONFIG,
  mergeAdminConfig,
} from '../../../hooks/common/useSidebar';
import {
  CUSTOM_NAV_ICON_OPTIONS,
  CUSTOM_NAV_SECTION_OPTIONS,
  createCustomNavItem,
  getCustomNavIcon,
  parseCustomNavItems,
} from '../../../helpers/customNav';

const { Text } = Typography;

const cloneDefaultAdminConfig = () => mergeAdminConfig(DEFAULT_ADMIN_CONFIG);

export default function SettingsSidebarModulesAdmin(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [statusState, statusDispatch] = useContext(StatusContext);
  const [sidebarModulesAdmin, setSidebarModulesAdmin] = useState(
    cloneDefaultAdminConfig,
  );

  function updateSidebarModules(updater) {
    setSidebarModulesAdmin((current) => updater(current));
  }

  function handleSectionChange(sectionKey) {
    return (checked) => {
      updateSidebarModules((current) => ({
        ...current,
        [sectionKey]: {
          ...current[sectionKey],
          enabled: checked,
        },
      }));
    };
  }

  function handleModuleChange(sectionKey, moduleKey) {
    return (checked) => {
      updateSidebarModules((current) => ({
        ...current,
        [sectionKey]: {
          ...current[sectionKey],
          [moduleKey]: checked,
        },
      }));
    };
  }

  function handleSectionFieldChange(sectionKey, fieldKey, value) {
    updateSidebarModules((current) => ({
      ...current,
      [sectionKey]: {
        ...current[sectionKey],
        [fieldKey]: value,
      },
    }));
  }

  function handleCustomItemChange(index, patch) {
    updateSidebarModules((current) => ({
      ...current,
      customItems: (current.customItems || []).map((item, currentIndex) =>
        currentIndex === index ? { ...item, ...patch } : item,
      ),
    }));
  }

  function addCustomItem() {
    updateSidebarModules((current) => ({
      ...current,
      customItems: [
        ...(current.customItems || []),
        createCustomNavItem('chat'),
      ],
    }));
  }

  function removeCustomItem(index) {
    updateSidebarModules((current) => ({
      ...current,
      customItems: (current.customItems || []).filter(
        (_, currentIndex) => currentIndex !== index,
      ),
    }));
  }

  function resetSidebarModules() {
    setSidebarModulesAdmin(cloneDefaultAdminConfig());
    showSuccess(t('已重置为默认配置'));
  }

  async function onSubmit() {
    setLoading(true);
    try {
      const normalizedModules = {
        ...sidebarModulesAdmin,
        customItems: parseCustomNavItems(sidebarModulesAdmin.customItems, {
          includeDisabled: true,
        }),
      };
      const serialized = JSON.stringify(normalizedModules);
      const res = await API.put('/api/option/', {
        key: 'SidebarModulesAdmin',
        value: serialized,
      });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('保存成功'));
        setSidebarModulesAdmin(normalizedModules);

        statusDispatch({
          type: 'set',
          payload: {
            ...statusState.status,
            SidebarModulesAdmin: serialized,
          },
        });

        if (props.refresh) {
          await props.refresh();
        }
      } else {
        showError(message);
      }
    } catch (error) {
      showError(t('保存失败，请重试'));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    if (props.options && props.options.SidebarModulesAdmin) {
      try {
        const modules = JSON.parse(props.options.SidebarModulesAdmin);
        setSidebarModulesAdmin(mergeAdminConfig(modules));
      } catch (error) {
        setSidebarModulesAdmin(cloneDefaultAdminConfig());
      }
    } else {
      setSidebarModulesAdmin(cloneDefaultAdminConfig());
    }
  }, [props.options]);

  const sectionConfigs = [
    {
      key: 'chat',
      title: t('聊天区域'),
      description: t('操练场、无限画布和聊天功能'),
      modules: [
        {
          key: 'playground',
          title: t('操练场'),
          description: t('AI模型测试环境'),
        },
        {
          key: 'canvas',
          title: t('无限画布'),
          description: t('选择分组后打开无限画布'),
        },
        { key: 'chat', title: t('聊天'), description: t('聊天会话管理') },
      ],
    },
    {
      key: 'console',
      title: t('控制台区域'),
      description: t('数据管理和日志查看'),
      modules: [
        { key: 'detail', title: t('数据看板'), description: t('系统数据统计') },
        { key: 'token', title: t('令牌管理'), description: t('API令牌管理') },
        { key: 'log', title: t('使用日志'), description: t('API使用记录') },
        {
          key: 'midjourney',
          title: t('绘图日志'),
          description: t('绘图任务记录'),
        },
        { key: 'task', title: t('任务日志'), description: t('系统任务记录') },
        {
          key: 'game_center',
          title: t('游戏中心'),
          description: t('游戏钱包、预测局和参与入口'),
        },
      ],
    },
    {
      key: 'personal',
      title: t('个人中心区域'),
      description: t('用户个人功能'),
      modules: [
        { key: 'topup', title: t('钱包管理'), description: t('余额充值管理') },
        {
          key: 'affiliate',
          title: t('返佣分成'),
          description: t('邀请返佣与提现管理'),
        },
        {
          key: 'invoice',
          title: t('发票中心'),
          description: t('用户发票申请与下载记录'),
        },
        {
          key: 'personal',
          title: t('个人设置'),
          description: t('个人信息设置'),
        },
      ],
    },
    {
      key: 'admin',
      title: t('管理员区域'),
      description: t('系统管理功能'),
      modules: [
        { key: 'channel', title: t('渠道管理'), description: t('API渠道配置') },
        { key: 'models', title: t('模型管理'), description: t('AI模型配置') },
        {
          key: 'deployment',
          title: t('模型部署'),
          description: t('模型部署管理'),
        },
        {
          key: 'subscription',
          title: t('订阅管理'),
          description: t('订阅套餐管理'),
        },
        {
          key: 'redemption',
          title: t('营销福利'),
          description: t('兑换码和优惠码管理'),
        },
        {
          key: 'game_management',
          title: t('游戏管理'),
          description: t('预测局创建与结算'),
        },
        { key: 'user', title: t('用户管理'), description: t('用户账户管理') },
        {
          key: 'invoice_admin',
          title: t('发票管理'),
          description: t('审核和处理发票工单'),
        },
        {
          key: 'affiliate_admin',
          title: t('返佣分成设置'),
          description: t('返佣配置与提现审核'),
        },
        {
          key: 'extension_admin',
          title: t('模块管理'),
          description: t('扩展模块管理与启停'),
        },
        {
          key: 'setting',
          title: t('系统设置'),
          description: t('系统参数配置'),
        },
      ],
    },
  ];

  const customItems = sidebarModulesAdmin.customItems || [];

  return (
    <Card>
      <Form.Section
        text={t('侧边栏管理（全局控制）')}
        extraText={t(
          '全局控制侧边栏区域和功能显示，管理员隐藏的功能用户无法启用',
        )}
      >
        {sectionConfigs.map((section) => (
          <div key={section.key} style={{ marginBottom: '32px' }}>
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                marginBottom: '16px',
                padding: '12px 16px',
                backgroundColor: 'var(--semi-color-fill-0)',
                borderRadius: '8px',
                border: '1px solid var(--semi-color-border)',
              }}
            >
              <div>
                <div
                  style={{
                    fontWeight: '600',
                    fontSize: '16px',
                    color: 'var(--semi-color-text-0)',
                    marginBottom: '4px',
                  }}
                >
                  {section.title}
                </div>
                <Text
                  type='secondary'
                  size='small'
                  style={{
                    fontSize: '12px',
                    color: 'var(--semi-color-text-2)',
                    lineHeight: '1.4',
                  }}
                >
                  {section.description}
                </Text>
              </div>
              <Switch
                checked={sidebarModulesAdmin[section.key]?.enabled}
                onChange={handleSectionChange(section.key)}
                size='default'
              />
            </div>

            <Row gutter={[16, 16]}>
              {section.modules.map((module) => (
                <Col key={module.key} xs={24} sm={12} md={8} lg={6} xl={6}>
                  <div
                    style={{
                      minHeight: '108px',
                      padding: '16px',
                      border: '1px solid var(--semi-color-border)',
                      borderRadius: '8px',
                      opacity: sidebarModulesAdmin[section.key]?.enabled
                        ? 1
                        : 0.5,
                      transition: 'opacity 0.2s',
                      backgroundColor: 'var(--semi-color-bg-0)',
                    }}
                  >
                    <div
                      style={{
                        display: 'flex',
                        justifyContent: 'space-between',
                        alignItems: 'center',
                        height: '100%',
                      }}
                    >
                      <div style={{ flex: 1, textAlign: 'left' }}>
                        <div
                          style={{
                            fontWeight: '600',
                            fontSize: '14px',
                            color: 'var(--semi-color-text-0)',
                            marginBottom: '4px',
                          }}
                        >
                          {module.title}
                        </div>
                        <Text
                          type='secondary'
                          size='small'
                          style={{
                            fontSize: '12px',
                            color: 'var(--semi-color-text-2)',
                            lineHeight: '1.4',
                            display: 'block',
                          }}
                        >
                          {module.description}
                        </Text>
                      </div>
                      <div style={{ marginLeft: '16px' }}>
                        <Switch
                          checked={
                            sidebarModulesAdmin[section.key]?.[module.key]
                          }
                          onChange={handleModuleChange(section.key, module.key)}
                          size='default'
                          disabled={!sidebarModulesAdmin[section.key]?.enabled}
                        />
                      </div>
                    </div>
                  </div>
                </Col>
              ))}
            </Row>

            {section.key === 'chat' && (
              <div
                style={{
                  marginTop: '16px',
                  padding: '16px',
                  border: '1px solid var(--semi-color-border)',
                  borderRadius: '8px',
                  backgroundColor: 'var(--semi-color-fill-0)',
                }}
              >
                <div
                  style={{
                    fontWeight: 600,
                    marginBottom: '12px',
                    color: 'var(--semi-color-text-0)',
                  }}
                >
                  {t('无限画布配置')}
                </div>
                <Row gutter={[16, 16]}>
                  <Col xs={24} md={14}>
                    <div style={{ marginBottom: '6px', fontSize: '13px' }}>
                      {t('画布应用域名')}
                    </div>
                    <Input
                      value={sidebarModulesAdmin.chat?.canvasOrigin || ''}
                      placeholder='https://canvas.example.com'
                      onChange={(value) =>
                        handleSectionFieldChange('chat', 'canvasOrigin', value)
                      }
                      disabled={
                        !sidebarModulesAdmin.chat?.enabled ||
                        !sidebarModulesAdmin.chat?.canvas
                      }
                    />
                    <Text type='secondary' size='small'>
                      {t('支持填写域名或完整 Origin，例如 canvas.example.com')}
                    </Text>
                  </Col>
                  <Col xs={24} md={10}>
                    <div style={{ marginBottom: '6px', fontSize: '13px' }}>
                      {t('画布图标')}
                    </div>
                    <div style={{ display: 'flex', gap: '8px' }}>
                      <div
                        style={{
                          width: 34,
                          height: 34,
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          borderRadius: 8,
                          border: '1px solid var(--semi-color-border)',
                          backgroundColor: 'var(--semi-color-bg-0)',
                        }}
                      >
                        {getCustomNavIcon(sidebarModulesAdmin.chat?.canvasIcon)}
                      </div>
                      <Select
                        value={sidebarModulesAdmin.chat?.canvasIcon || 'Brush'}
                        onChange={(value) =>
                          handleSectionFieldChange('chat', 'canvasIcon', value)
                        }
                        style={{ flex: 1 }}
                        disabled={
                          !sidebarModulesAdmin.chat?.enabled ||
                          !sidebarModulesAdmin.chat?.canvas
                        }
                      >
                        {CUSTOM_NAV_ICON_OPTIONS.map((iconName) => (
                          <Select.Option key={iconName} value={iconName}>
                            {iconName}
                          </Select.Option>
                        ))}
                      </Select>
                    </div>
                  </Col>
                </Row>
              </div>
            )}
          </div>
        ))}

        <div
          style={{
            marginBottom: '24px',
            padding: '16px',
            border: '1px solid var(--semi-color-border)',
            borderRadius: '8px',
            backgroundColor: 'var(--semi-color-bg-0)',
          }}
        >
          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              gap: '12px',
              marginBottom: '12px',
            }}
          >
            <div>
              <div
                style={{
                  fontWeight: 600,
                  color: 'var(--semi-color-text-0)',
                  marginBottom: '4px',
                }}
              >
                {t('自定义侧边栏')}
              </div>
              <Text type='secondary' size='small'>
                {t('添加无需改代码的侧边栏链接，可选择显示区域和图标')}
              </Text>
            </div>
            <Button icon={<IconPlus />} onClick={addCustomItem}>
              {t('添加自定义项')}
            </Button>
          </div>

          {customItems.length === 0 ? (
            <Text type='secondary' size='small'>
              {t('暂无自定义侧边栏项')}
            </Text>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              {customItems.map((item, index) => (
                <div
                  key={item.id || index}
                  style={{
                    padding: '14px',
                    border: '1px solid var(--semi-color-border)',
                    borderRadius: '8px',
                    backgroundColor: 'var(--semi-color-fill-0)',
                  }}
                >
                  <div
                    style={{
                      display: 'flex',
                      justifyContent: 'space-between',
                      alignItems: 'center',
                      gap: 12,
                      marginBottom: 12,
                    }}
                  >
                    <div
                      style={{
                        display: 'flex',
                        alignItems: 'center',
                        gap: 8,
                        minWidth: 0,
                      }}
                    >
                      {getCustomNavIcon(item.icon)}
                      <span
                        style={{
                          fontWeight: 600,
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                          whiteSpace: 'nowrap',
                        }}
                      >
                        {item.title || t('未命名菜单项')}
                      </span>
                    </div>
                    <Button
                      icon={<IconDelete />}
                      type='danger'
                      theme='borderless'
                      onClick={() => removeCustomItem(index)}
                    />
                  </div>

                  <Row gutter={[12, 12]}>
                    <Col xs={24} md={8}>
                      <div style={{ marginBottom: 6, fontSize: 13 }}>
                        {t('标题')}
                      </div>
                      <Input
                        value={item.title}
                        onChange={(value) =>
                          handleCustomItemChange(index, { title: value })
                        }
                      />
                    </Col>
                    <Col xs={24} md={10}>
                      <div style={{ marginBottom: 6, fontSize: 13 }}>
                        {t('链接')}
                      </div>
                      <Input
                        value={item.url}
                        placeholder='/console/channel 或 https://example.com'
                        onChange={(value) =>
                          handleCustomItemChange(index, { url: value })
                        }
                      />
                    </Col>
                    <Col xs={24} md={6}>
                      <div style={{ marginBottom: 6, fontSize: 13 }}>
                        {t('显示区域')}
                      </div>
                      <Select
                        value={item.section || 'chat'}
                        style={{ width: '100%' }}
                        onChange={(value) =>
                          handleCustomItemChange(index, { section: value })
                        }
                      >
                        {CUSTOM_NAV_SECTION_OPTIONS.map((option) => (
                          <Select.Option
                            key={option.value}
                            value={option.value}
                          >
                            {t(option.label)}
                          </Select.Option>
                        ))}
                      </Select>
                    </Col>
                    <Col xs={24} md={8}>
                      <div style={{ marginBottom: 6, fontSize: 13 }}>
                        {t('图标')}
                      </div>
                      <Select
                        value={item.icon || 'ExternalLink'}
                        style={{ width: '100%' }}
                        onChange={(value) =>
                          handleCustomItemChange(index, { icon: value })
                        }
                      >
                        {CUSTOM_NAV_ICON_OPTIONS.map((iconName) => (
                          <Select.Option key={iconName} value={iconName}>
                            {iconName}
                          </Select.Option>
                        ))}
                      </Select>
                    </Col>
                    <Col xs={24} md={5}>
                      <div style={{ marginBottom: 6, fontSize: 13 }}>
                        {t('排序')}
                      </div>
                      <InputNumber
                        value={item.order || 0}
                        min={0}
                        style={{ width: '100%' }}
                        onChange={(value) =>
                          handleCustomItemChange(index, {
                            order: Number(value) || 0,
                          })
                        }
                      />
                    </Col>
                    <Col xs={24} md={11}>
                      <div
                        style={{
                          display: 'flex',
                          gap: 20,
                          alignItems: 'center',
                          height: '100%',
                          paddingTop: 24,
                        }}
                      >
                        <label
                          style={{
                            display: 'flex',
                            alignItems: 'center',
                            gap: 8,
                          }}
                        >
                          <Switch
                            checked={item.enabled !== false}
                            onChange={(enabled) =>
                              handleCustomItemChange(index, { enabled })
                            }
                          />
                          <span>{t('启用')}</span>
                        </label>
                        <label
                          style={{
                            display: 'flex',
                            alignItems: 'center',
                            gap: 8,
                          }}
                        >
                          <Switch
                            checked={Boolean(item.openInNewTab)}
                            onChange={(openInNewTab) =>
                              handleCustomItemChange(index, { openInNewTab })
                            }
                          />
                          <span>{t('新窗口打开')}</span>
                        </label>
                      </div>
                    </Col>
                  </Row>
                </div>
              ))}
            </div>
          )}
        </div>

        <div
          style={{
            display: 'flex',
            gap: '12px',
            justifyContent: 'flex-start',
            alignItems: 'center',
            paddingTop: '8px',
            borderTop: '1px solid var(--semi-color-border)',
          }}
        >
          <Button
            size='default'
            type='tertiary'
            onClick={resetSidebarModules}
            style={{
              borderRadius: '6px',
              fontWeight: '500',
            }}
          >
            {t('重置为默认')}
          </Button>
          <Button
            size='default'
            type='primary'
            onClick={onSubmit}
            loading={loading}
            style={{
              borderRadius: '6px',
              fontWeight: '500',
              minWidth: '100px',
            }}
          >
            {t('保存设置')}
          </Button>
        </div>
      </Form.Section>
    </Card>
  );
}
