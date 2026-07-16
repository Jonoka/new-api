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
  Card,
  Checkbox,
  Col,
  Form,
  InputNumber,
  Row,
  Select,
  Space,
  Spin,
  Table,
  Typography,
} from '@douyinfe/semi-ui';
import {
  API,
  removeTrailingSlash,
  showError,
  showSuccess,
  verifyJSON,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

const DEFAULT_INVOICE_TYPES = '["personal","company"]';
const DEFAULT_INVOICE_FEE_RULES =
  '[{"min":0,"max":500,"type":"fixed","value":50},{"min":501,"max":2000,"type":"fixed","value":100},{"min":2001,"max":5000,"type":"fixed","value":175},{"min":5000,"type":"percent","value":5}]';

const parseInvoiceTypes = (value) => {
  try {
    const parsed = JSON.parse(value || '[]');
    if (!Array.isArray(parsed)) return ['personal', 'company'];
    const types = parsed.filter(
      (item) => item === 'personal' || item === 'company',
    );
    return types.length > 0 ? types : ['personal', 'company'];
  } catch {
    return ['personal', 'company'];
  }
};

const parseInvoiceRules = (value) => {
  try {
    const parsed = JSON.parse(value || '[]');
    if (!Array.isArray(parsed))
      return parseInvoiceRules(DEFAULT_INVOICE_FEE_RULES);
    const rules = parsed
      .map((item) => ({
        min: Number(item?.min ?? 0),
        max:
          item?.max === undefined || item?.max === ''
            ? undefined
            : Number(item.max),
        type: item?.type === 'percent' ? 'percent' : 'fixed',
        value: Number(item?.value ?? 0),
        max_fee:
          item?.max_fee === undefined || item?.max_fee === ''
            ? undefined
            : Number(item.max_fee),
      }))
      .filter(
        (rule) =>
          Number.isFinite(rule.min) &&
          (rule.max === undefined || Number.isFinite(rule.max)) &&
          Number.isFinite(rule.value) &&
          (rule.max_fee === undefined || Number.isFinite(rule.max_fee)),
      )
      .sort((a, b) => a.min - b.min);
    return rules.length > 0
      ? rules
      : parseInvoiceRules(DEFAULT_INVOICE_FEE_RULES);
  } catch {
    return [
      { min: 0, max: 500, type: 'fixed', value: 50 },
      { min: 501, max: 2000, type: 'fixed', value: 100 },
      { min: 2001, max: 5000, type: 'fixed', value: 175 },
      { min: 5000, type: 'percent', value: 5 },
    ];
  }
};

const serializeInvoiceTypes = (types) =>
  JSON.stringify(types.length > 0 ? types : ['personal'], null, 2);

const serializeInvoiceRules = (rules) =>
  JSON.stringify(
    rules
      .map((rule) => ({
        min: Number(rule.min) || 0,
        ...(rule.max !== undefined && Number(rule.max) > 0
          ? { max: Number(rule.max) }
          : {}),
        type: rule.type === 'percent' ? 'percent' : 'fixed',
        value: Number(rule.value) || 0,
        ...(rule.type === 'percent' &&
        rule.max_fee !== undefined &&
        Number(rule.max_fee) > 0
          ? { max_fee: Number(rule.max_fee) }
          : {}),
      }))
      .sort((a, b) => a.min - b.min),
    null,
    2,
  );

const InvoiceSettingsVisualEditor = ({
  t,
  typesValue,
  rulesValue,
  onChange,
}) => {
  const types = parseInvoiceTypes(typesValue);
  const rules = parseInvoiceRules(rulesValue);

  const updateTypes = (nextTypes) => {
    onChange({
      InvoiceTypes: serializeInvoiceTypes(nextTypes),
      InvoiceFeeRules: rulesValue,
    });
  };

  const updateRules = (nextRules) => {
    onChange({
      InvoiceTypes: typesValue,
      InvoiceFeeRules: serializeInvoiceRules(nextRules),
    });
  };

  const updateTypeSelection = (nextTypes) => {
    updateTypes(nextTypes.length > 0 ? nextTypes : types);
  };

  const patchRule = (index, patch) => {
    const next = rules.map((rule, idx) =>
      idx === index ? { ...rule, ...patch } : rule,
    );
    updateRules(next);
  };

  const addRule = () => {
    const last = rules[rules.length - 1];
    const nextMin = last?.max ? last.max + 1 : (last?.min || 0) + 1;
    updateRules([...rules, { min: nextMin, type: 'fixed', value: 0 }]);
  };

  const deleteRule = (index) => {
    const next = rules.filter((_, idx) => idx !== index);
    updateRules(
      next.length > 0 ? next : parseInvoiceRules(DEFAULT_INVOICE_FEE_RULES),
    );
  };

  const columns = [
    {
      title: t('最小金额'),
      dataIndex: 'min',
      render: (_, record, index) => (
        <InputNumber
          min={0}
          value={record.min}
          onChange={(value) => patchRule(index, { min: Number(value) || 0 })}
          style={{ width: 120 }}
        />
      ),
    },
    {
      title: t('最大金额'),
      dataIndex: 'max',
      render: (_, record, index) => (
        <InputNumber
          min={0}
          value={record.max}
          placeholder={t('无上限')}
          onChange={(value) =>
            patchRule(index, {
              max:
                value === undefined || value === null || value === ''
                  ? undefined
                  : Number(value),
            })
          }
          style={{ width: 120 }}
        />
      ),
    },
    {
      title: t('收费方式'),
      dataIndex: 'type',
      render: (_, record, index) => (
        <Select
          value={record.type}
          onChange={(value) =>
            patchRule(index, {
              type: value,
              ...(value === 'percent' ? {} : { max_fee: undefined }),
            })
          }
          style={{ width: 130 }}
          optionList={[
            { value: 'fixed', label: t('固定金额') },
            { value: 'percent', label: t('百分比') },
          ]}
        />
      ),
    },
    {
      title: t('收费值'),
      dataIndex: 'value',
      render: (_, record, index) => (
        <InputNumber
          min={0}
          value={record.value}
          onChange={(value) => patchRule(index, { value: Number(value) || 0 })}
          style={{ width: 120 }}
        />
      ),
    },
    {
      title: t('收费上限'),
      dataIndex: 'max_fee',
      render: (_, record, index) => (
        <InputNumber
          min={0}
          value={record.type === 'percent' ? record.max_fee : undefined}
          placeholder={t('无封顶')}
          disabled={record.type !== 'percent'}
          onChange={(value) =>
            patchRule(index, {
              max_fee:
                value === undefined || value === null || value === ''
                  ? undefined
                  : Number(value),
            })
          }
          style={{ width: 120 }}
        />
      ),
    },
    {
      title: t('操作'),
      render: (_, __, index) => (
        <Button
          type='danger'
          theme='borderless'
          onClick={() => deleteRule(index)}
        >
          {t('删除')}
        </Button>
      ),
    },
  ];

  return (
    <Card bodyStyle={{ padding: 16 }}>
      <Space vertical align='start' style={{ width: '100%' }}>
        <div>
          <Typography.Text strong>{t('发票类型')}</Typography.Text>
          <div style={{ marginTop: 8 }}>
            <Checkbox.Group value={types} onChange={updateTypeSelection}>
              <Checkbox value='personal'>{t('对私')}</Checkbox>
              <Checkbox value='company' style={{ marginLeft: 16 }}>
                {t('对公')}
              </Checkbox>
            </Checkbox.Group>
          </div>
        </div>

        <div style={{ width: '100%' }}>
          <div
            className='flex items-center justify-between'
            style={{ marginBottom: 8 }}
          >
            <div>
              <Typography.Text strong>{t('发票费用规则')}</Typography.Text>
              <div style={{ color: 'var(--semi-color-text-2)', fontSize: 12 }}>
                {t('费用按人民币计算。最大金额留空表示无上限')}
              </div>
            </div>
            <Button onClick={addRule}>{t('新增规则')}</Button>
          </div>
          <Table
            columns={columns}
            dataSource={rules.map((rule, index) => ({ ...rule, key: index }))}
            pagination={false}
            size='small'
          />
        </div>
      </Space>
    </Card>
  );
};

export default function SettingsGeneralPayment(props) {
  const { t } = useTranslation();
  const sectionTitle = props.hideSectionTitle ? undefined : t('通用设置');
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    ServerAddress: '',
    CustomCallbackAddress: '',
    TopupGroupRatio: '',
    PayMethods: '',
    AmountOptions: '',
    AmountDiscount: '',
    'payment_setting.balance_subscription_enabled': true,
    'payment_setting.balance_subscription_promo_enabled': true,
    InvoiceEnabled: false,
    InvoiceTypes: DEFAULT_INVOICE_TYPES,
    InvoiceFeeRules: DEFAULT_INVOICE_FEE_RULES,
  });
  const [originInputs, setOriginInputs] = useState({});
  const formApiRef = useRef(null);

  useEffect(() => {
    if (props.options && formApiRef.current) {
      const currentInputs = {
        ServerAddress: props.options.ServerAddress || '',
        CustomCallbackAddress: props.options.CustomCallbackAddress || '',
        TopupGroupRatio: props.options.TopupGroupRatio || '',
        PayMethods: props.options.PayMethods || '',
        AmountOptions: props.options.AmountOptions || '',
        AmountDiscount: props.options.AmountDiscount || '',
        'payment_setting.balance_subscription_enabled':
          props.options['payment_setting.balance_subscription_enabled'] !==
          false,
        'payment_setting.balance_subscription_promo_enabled':
          props.options[
            'payment_setting.balance_subscription_promo_enabled'
          ] !== false,
        InvoiceEnabled: !!props.options.InvoiceEnabled,
        InvoiceTypes: props.options.InvoiceTypes || DEFAULT_INVOICE_TYPES,
        InvoiceFeeRules:
          props.options.InvoiceFeeRules || DEFAULT_INVOICE_FEE_RULES,
      };
      setInputs(currentInputs);
      setOriginInputs({ ...currentInputs });
      formApiRef.current.setValues(currentInputs);
    }
  }, [props.options]);

  const handleFormChange = ({ InvoiceTypes, InvoiceFeeRules, ...values }) => {
    setInputs((prev) => ({
      ...prev,
      ...values,
    }));
  };

  const submitGeneralSettings = async () => {
    if (
      originInputs.TopupGroupRatio !== inputs.TopupGroupRatio &&
      !verifyJSON(inputs.TopupGroupRatio)
    ) {
      showError(t('充值分组倍率不是合法的 JSON 字符串'));
      return;
    }

    if (
      originInputs.PayMethods !== inputs.PayMethods &&
      !verifyJSON(inputs.PayMethods)
    ) {
      showError(t('充值方式设置不是合法的 JSON 字符串'));
      return;
    }

    if (
      originInputs.AmountOptions !== inputs.AmountOptions &&
      inputs.AmountOptions.trim() !== '' &&
      !verifyJSON(inputs.AmountOptions)
    ) {
      showError(t('自定义充值数量选项不是合法的 JSON 数组'));
      return;
    }

    if (
      originInputs.AmountDiscount !== inputs.AmountDiscount &&
      inputs.AmountDiscount.trim() !== '' &&
      !verifyJSON(inputs.AmountDiscount)
    ) {
      showError(t('充值金额折扣配置不是合法的 JSON 对象'));
      return;
    }

    setLoading(true);
    try {
      const options = [
        {
          key: 'ServerAddress',
          value: removeTrailingSlash(inputs.ServerAddress),
        },
      ];

      if (inputs.CustomCallbackAddress !== '') {
        options.push({
          key: 'CustomCallbackAddress',
          value: removeTrailingSlash(inputs.CustomCallbackAddress),
        });
      }
      if (originInputs.TopupGroupRatio !== inputs.TopupGroupRatio) {
        options.push({ key: 'TopupGroupRatio', value: inputs.TopupGroupRatio });
      }
      if (originInputs.PayMethods !== inputs.PayMethods) {
        options.push({ key: 'PayMethods', value: inputs.PayMethods });
      }
      if (originInputs.AmountOptions !== inputs.AmountOptions) {
        options.push({
          key: 'payment_setting.amount_options',
          value: inputs.AmountOptions,
        });
      }
      if (originInputs.AmountDiscount !== inputs.AmountDiscount) {
        options.push({
          key: 'payment_setting.amount_discount',
          value: inputs.AmountDiscount,
        });
      }
      if (
        originInputs['payment_setting.balance_subscription_enabled'] !==
        inputs['payment_setting.balance_subscription_enabled']
      ) {
        options.push({
          key: 'payment_setting.balance_subscription_enabled',
          value: inputs['payment_setting.balance_subscription_enabled'],
        });
      }
      if (
        originInputs['payment_setting.balance_subscription_promo_enabled'] !==
        inputs['payment_setting.balance_subscription_promo_enabled']
      ) {
        options.push({
          key: 'payment_setting.balance_subscription_promo_enabled',
          value: inputs['payment_setting.balance_subscription_promo_enabled'],
        });
      }
      if (originInputs.InvoiceEnabled !== inputs.InvoiceEnabled) {
        options.push({
          key: 'InvoiceEnabled',
          value: inputs.InvoiceEnabled,
        });
      }
      if (originInputs.InvoiceTypes !== inputs.InvoiceTypes) {
        options.push({ key: 'InvoiceTypes', value: inputs.InvoiceTypes });
      }
      if (originInputs.InvoiceFeeRules !== inputs.InvoiceFeeRules) {
        options.push({
          key: 'InvoiceFeeRules',
          value: inputs.InvoiceFeeRules,
        });
      }

      const results = await Promise.all(
        options.map((option) =>
          API.put('/api/option/', {
            key: option.key,
            value: option.value,
          }),
        ),
      );

      const errorResults = results.filter((res) => !res.data.success);
      if (errorResults.length === 0) {
        showSuccess(t('更新成功'));
        setOriginInputs({ ...inputs });
        props.refresh && props.refresh();
      } else {
        errorResults.forEach((res) => {
          showError(res.data.message);
        });
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
          <Form.Input
            field='ServerAddress'
            label={t('服务器地址')}
            placeholder={'https://yourdomain.com'}
            style={{ width: '100%' }}
            extraText={t(
              '该服务器地址将影响支付回调地址以及默认首页展示的地址，请确保正确配置',
            )}
          />
          <Row
            gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
            style={{ marginTop: 16 }}
          >
            <Col xs={24} sm={24} md={12} lg={12} xl={12}>
              <Form.Input
                field='CustomCallbackAddress'
                label={t('回调地址')}
                placeholder={t('例如：https://yourdomain.com')}
                extraText={t(
                  '留空时默认使用服务器地址作为回调地址，填写后将覆盖默认值',
                )}
              />
            </Col>
            <Col xs={24} sm={24} md={12} lg={12} xl={12}>
              <Form.TextArea
                field='TopupGroupRatio'
                label={t('充值分组倍率')}
                placeholder={t('为一个 JSON 文本，键为组名称，值为倍率')}
                autosize
              />
            </Col>
          </Row>
          <Row
            gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
            style={{ marginTop: 16 }}
          >
            <Col xs={24} sm={24} md={12} lg={12} xl={12}>
              <Form.TextArea
                field='PayMethods'
                label={t('充值方式设置')}
                placeholder={t('为一个 JSON 文本')}
                autosize
              />
            </Col>
            <Col xs={24} sm={24} md={12} lg={12} xl={12}>
              <Form.TextArea
                field='AmountOptions'
                label={t('自定义充值数量选项')}
                placeholder={t(
                  '为一个 JSON 数组，例如：[10, 20, 50, 100, 200, 500]',
                )}
                autosize
                extraText={t(
                  '设置用户可选择的充值数量选项，例如：[10, 20, 50, 100, 200, 500]',
                )}
              />
            </Col>
          </Row>
          <Row style={{ marginTop: 16 }}>
            <Col span={24}>
              <Form.TextArea
                field='AmountDiscount'
                label={t('充值金额折扣配置')}
                placeholder={t(
                  '为一个 JSON 对象，例如：{"100": 0.95, "200": 0.9, "500": 0.85}',
                )}
                autosize
                extraText={t(
                  '设置不同充值金额对应的折扣，键为充值金额，值为折扣率，例如：{"100": 0.95, "200": 0.9, "500": 0.85}',
                )}
              />
            </Col>
          </Row>
          <Row
            gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
            style={{ marginTop: 16 }}
          >
            <Col xs={24} sm={24} md={12} lg={12} xl={12}>
              <Form.Switch
                field='payment_setting.balance_subscription_enabled'
                label={t('余额购买订阅')}
                checkedText={t('开')}
                uncheckedText={t('关')}
                extraText={t('允许用户使用账户余额购买订阅套餐')}
              />
            </Col>
            <Col xs={24} sm={24} md={12} lg={12} xl={12}>
              <Form.Switch
                field='payment_setting.balance_subscription_promo_enabled'
                label={t('余额购买订阅使用优惠码')}
                checkedText={t('开')}
                uncheckedText={t('关')}
                disabled={
                  !inputs['payment_setting.balance_subscription_enabled']
                }
                extraText={t('允许余额购买订阅时使用优惠码')}
              />
            </Col>
          </Row>
          <Row
            gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
            style={{ marginTop: 16 }}
          >
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Switch
                field='InvoiceEnabled'
                label={t('发票开关')}
                checkedText={t('开')}
                uncheckedText={t('关')}
                extraText={t('开启后，充值和购买订阅时可选择申请发票')}
              />
            </Col>
            <Col xs={24} sm={24} md={16} lg={16} xl={16}>
              <InvoiceSettingsVisualEditor
                t={t}
                typesValue={inputs.InvoiceTypes}
                rulesValue={inputs.InvoiceFeeRules}
                onChange={(patch) => {
                  setInputs((prev) => ({
                    ...prev,
                    ...patch,
                  }));
                }}
              />
            </Col>
          </Row>
          <Button onClick={submitGeneralSettings} style={{ marginTop: 16 }}>
            {t('保存通用设置')}
          </Button>
        </Form.Section>
      </Form>
    </Spin>
  );
}
