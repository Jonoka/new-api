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

import React, { useEffect, useState, useRef } from 'react';
import {
  Button,
  Form,
  Row,
  Col,
  Spin,
  Space,
  Typography,
  Card,
} from '@douyinfe/semi-ui';
import { IconPlus, IconDelete } from '@douyinfe/semi-icons';
import {
  API,
  removeTrailingSlash,
  showError,
  showSuccess,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

const { Text } = Typography;

const PRESET_CHAINS = [
  { name: 'TRC20', trade_type: 'usdt.trc20' },
  { name: 'ERC20', trade_type: 'usdt.erc20' },
  { name: 'BEP20', trade_type: 'usdt.bep20' },
  { name: 'Polygon', trade_type: 'usdt.polygon' },
  { name: 'Arbitrum', trade_type: 'usdt.arbitrum' },
  { name: 'Solana', trade_type: 'usdt.solana' },
  { name: 'TON', trade_type: 'usdt.ton' },
];

export default function SettingsPaymentGatewayBepusdt(props) {
  const { t } = useTranslation();
  const sectionTitle = props.hideSectionTitle
    ? undefined
    : t('Bepusdt (USDT) 设置');
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    BepusdtApiUrl: '',
    BepusdtAuthToken: '',
    BepusdtUnitPrice: 7.2,
    BepusdtMinTopUp: 1,
    BepusdtTimeout: 1200,
  });
  const [chains, setChains] = useState([]);
  const formApiRef = useRef(null);

  useEffect(() => {
    if (props.options && formApiRef.current) {
      const currentInputs = {
        BepusdtApiUrl: props.options.BepusdtApiUrl || '',
        BepusdtAuthToken: props.options.BepusdtAuthToken || '',
        BepusdtUnitPrice:
          props.options.BepusdtUnitPrice !== undefined
            ? parseFloat(props.options.BepusdtUnitPrice)
            : 7.2,
        BepusdtMinTopUp:
          props.options.BepusdtMinTopUp !== undefined
            ? parseInt(props.options.BepusdtMinTopUp)
            : 1,
        BepusdtTimeout:
          props.options.BepusdtTimeout !== undefined
            ? parseInt(props.options.BepusdtTimeout)
            : 1200,
      };
      setInputs(currentInputs);
      formApiRef.current.setValues(currentInputs);

      // 解析链配置
      try {
        const parsed = JSON.parse(props.options.BepusdtChains || '[]');
        if (Array.isArray(parsed)) {
          setChains(parsed);
        }
      } catch {
        setChains([]);
      }
    }
  }, [props.options]);

  const handleFormChange = (values) => {
    setInputs(values);
  };

  const addChain = (preset) => {
    // 检查是否已存在
    if (chains.some((c) => c.trade_type === preset.trade_type)) {
      showError(t('该链已添加'));
      return;
    }
    setChains([
      ...chains,
      { name: preset.name, trade_type: preset.trade_type },
    ]);
  };

  const addCustomChain = () => {
    setChains([...chains, { name: '', trade_type: '' }]);
  };

  const removeChain = (index) => {
    setChains(chains.filter((_, i) => i !== index));
  };

  const updateChain = (index, field, value) => {
    const updated = [...chains];
    updated[index] = { ...updated[index], [field]: value };
    setChains(updated);
  };

  const submitBepusdtSetting = async () => {
    setLoading(true);
    try {
      const options = [
        {
          key: 'BepusdtApiUrl',
          value: removeTrailingSlash(inputs.BepusdtApiUrl),
        },
      ];

      if (
        inputs.BepusdtAuthToken !== undefined &&
        inputs.BepusdtAuthToken !== ''
      ) {
        options.push({
          key: 'BepusdtAuthToken',
          value: inputs.BepusdtAuthToken,
        });
      }
      if (inputs.BepusdtUnitPrice !== '') {
        options.push({
          key: 'BepusdtUnitPrice',
          value: inputs.BepusdtUnitPrice.toString(),
        });
      }
      if (inputs.BepusdtMinTopUp !== '') {
        options.push({
          key: 'BepusdtMinTopUp',
          value: inputs.BepusdtMinTopUp.toString(),
        });
      }
      if (inputs.BepusdtTimeout !== '') {
        options.push({
          key: 'BepusdtTimeout',
          value: inputs.BepusdtTimeout.toString(),
        });
      }

      // 过滤掉不完整的链
      const validChains = chains.filter(
        (c) => c.name.trim() !== '' && c.trade_type.trim() !== '',
      );
      options.push({
        key: 'BepusdtChains',
        value: JSON.stringify(validChains),
      });

      const requestQueue = options.map((opt) =>
        API.put('/api/option/', {
          key: opt.key,
          value: opt.value,
        }),
      );

      const results = await Promise.all(requestQueue);

      const errorResults = results.filter((res) => !res.data.success);
      if (errorResults.length > 0) {
        errorResults.forEach((res) => {
          showError(res.data.message);
        });
      } else {
        showSuccess(t('更新成功'));
        props.refresh && props.refresh();
      }
    } catch (error) {
      showError(t('更新失败'));
    }
    setLoading(false);
  };

  return (
    <Spin spinning={loading}>
      <Form
        initValues={inputs}
        onValueChange={handleFormChange}
        getFormApi={(api) => (formApiRef.current = api)}
      >
        <Form.Section text={sectionTitle}>
          <Row gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='BepusdtApiUrl'
                label={t('API 地址')}
                placeholder='https://usdt.example.com'
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='BepusdtAuthToken'
                label={t('认证令牌')}
                placeholder={t('敏感信息不会发送到前端显示')}
                type='password'
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.InputNumber
                field='BepusdtUnitPrice'
                precision={2}
                label={t('充值价格（x元/美金）')}
                placeholder={t('例如：7，就是7元/美金')}
              />
            </Col>
          </Row>
          <Row
            gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
            style={{ marginTop: 16 }}
          >
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.InputNumber
                field='BepusdtMinTopUp'
                label={t('最低充值美元数量')}
                placeholder={t('例如：2，就是最低充值2$')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.InputNumber
                field='BepusdtTimeout'
                label={t('订单超时（秒）')}
                placeholder={t('例如：1200')}
              />
            </Col>
          </Row>

          {/* 链配置可视化编辑器 */}
          <div style={{ marginTop: 24 }}>
            <Text strong style={{ display: 'block', marginBottom: 8 }}>
              {t('支持的链')}
            </Text>
            <Text
              type='tertiary'
              size='small'
              style={{ display: 'block', marginBottom: 12 }}
            >
              {t('点击快捷按钮添加常用链，或手动添加自定义链')}
            </Text>

            {/* 快捷添加按钮 */}
            <Space wrap style={{ marginBottom: 12 }}>
              {PRESET_CHAINS.map((preset) => {
                const exists = chains.some(
                  (c) => c.trade_type === preset.trade_type,
                );
                return (
                  <Button
                    key={preset.trade_type}
                    size='small'
                    theme='light'
                    type={exists ? 'primary' : 'tertiary'}
                    disabled={exists}
                    icon={!exists ? <IconPlus /> : undefined}
                    onClick={() => addChain(preset)}
                  >
                    {preset.name}
                  </Button>
                );
              })}
              <Button
                size='small'
                theme='outline'
                icon={<IconPlus />}
                onClick={addCustomChain}
              >
                {t('自定义')}
              </Button>
            </Space>

            {/* 已添加的链列表 */}
            {chains.length > 0 && (
              <Card bodyStyle={{ padding: 12 }}>
                <Space vertical align='start' style={{ width: '100%' }}>
                  {chains.map((chain, index) => (
                    <Row
                      key={index}
                      gutter={8}
                      type='flex'
                      align='middle'
                      style={{ width: '100%' }}
                    >
                      <Col span={8}>
                        <input
                          style={{
                            width: '100%',
                            padding: '4px 8px',
                            border: '1px solid var(--semi-color-border)',
                            borderRadius: 4,
                            fontSize: 13,
                          }}
                          placeholder={t('显示名称')}
                          value={chain.name}
                          onChange={(e) =>
                            updateChain(index, 'name', e.target.value)
                          }
                        />
                      </Col>
                      <Col span={12}>
                        <input
                          style={{
                            width: '100%',
                            padding: '4px 8px',
                            border: '1px solid var(--semi-color-border)',
                            borderRadius: 4,
                            fontSize: 13,
                          }}
                          placeholder={t('trade_type (如 usdt.trc20)')}
                          value={chain.trade_type}
                          onChange={(e) =>
                            updateChain(index, 'trade_type', e.target.value)
                          }
                        />
                      </Col>
                      <Col span={4}>
                        <Button
                          icon={<IconDelete />}
                          type='danger'
                          theme='borderless'
                          size='small'
                          onClick={() => removeChain(index)}
                        />
                      </Col>
                    </Row>
                  ))}
                </Space>
              </Card>
            )}

            {chains.length === 0 && (
              <Text type='tertiary' size='small'>
                {t('未添加任何链，请点击上方按钮添加')}
              </Text>
            )}
          </div>

          <Button onClick={submitBepusdtSetting} style={{ marginTop: 16 }}>
            {t('更新 Bepusdt 设置')}
          </Button>
        </Form.Section>
      </Form>
    </Spin>
  );
}
