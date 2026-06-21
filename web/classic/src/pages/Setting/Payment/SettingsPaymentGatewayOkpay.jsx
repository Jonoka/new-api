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
import { Button, Form, Row, Col, Spin } from '@douyinfe/semi-ui';
import {
  API,
  removeTrailingSlash,
  showError,
  showSuccess,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

export default function SettingsPaymentGatewayOkpay(props) {
  const { t } = useTranslation();
  const sectionTitle = props.hideSectionTitle ? undefined : t('OKPay 设置');
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    OkpayGatewayUrl: '',
    OkpayMerchantId: '',
    OkpayMerchantToken: '',
    OkpayExchangeRate: 7.3,
    OkpayAutoExchangeEnabled: true,
    OkpayUsdtCnyRate: 7.2,
    OkpayRateApiUrl:
      'https://api.coingecko.com/api/v3/simple/price?ids=tether&vs_currencies=cny&include_last_updated_at=true',
    OkpayMinTopUp: 1,
    OkpayCoin: 'USDT',
  });
  const formApiRef = useRef(null);

  useEffect(() => {
    if (props.options && formApiRef.current) {
      const currentInputs = {
        OkpayGatewayUrl: props.options.OkpayGatewayUrl || '',
        OkpayMerchantId: props.options.OkpayMerchantId || '',
        OkpayMerchantToken: props.options.OkpayMerchantToken || '',
        OkpayExchangeRate:
          props.options.OkpayExchangeRate !== undefined
            ? parseFloat(props.options.OkpayExchangeRate)
            : 7.3,
        OkpayAutoExchangeEnabled:
          props.options.OkpayAutoExchangeEnabled !== undefined
            ? props.options.OkpayAutoExchangeEnabled === true ||
              props.options.OkpayAutoExchangeEnabled === 'true'
            : true,
        OkpayUsdtCnyRate:
          props.options.OkpayUsdtCnyRate !== undefined
            ? parseFloat(props.options.OkpayUsdtCnyRate)
            : 7.2,
        OkpayRateApiUrl:
          props.options.OkpayRateApiUrl ||
          'https://api.coingecko.com/api/v3/simple/price?ids=tether&vs_currencies=cny&include_last_updated_at=true',
        OkpayMinTopUp:
          props.options.OkpayMinTopUp !== undefined
            ? parseInt(props.options.OkpayMinTopUp)
            : 1,
        OkpayCoin: props.options.OkpayCoin || 'USDT',
      };

      setInputs(currentInputs);
      formApiRef.current.setValues(currentInputs);
    }
  }, [props.options]);

  const handleFormChange = (values) => {
    setInputs(values);
  };

  const submitOkpaySetting = async () => {
    setLoading(true);
    try {
      const options = [
        {
          key: 'OkpayGatewayUrl',
          value: removeTrailingSlash(inputs.OkpayGatewayUrl),
        },
      ];

      if (inputs.OkpayMerchantId !== '') {
        options.push({ key: 'OkpayMerchantId', value: inputs.OkpayMerchantId });
      }
      if (
        inputs.OkpayMerchantToken !== undefined &&
        inputs.OkpayMerchantToken !== ''
      ) {
        options.push({
          key: 'OkpayMerchantToken',
          value: inputs.OkpayMerchantToken,
        });
      }
      if (inputs.OkpayExchangeRate !== '') {
        options.push({
          key: 'OkpayExchangeRate',
          value: inputs.OkpayExchangeRate.toString(),
        });
      }
      options.push({
        key: 'OkpayAutoExchangeEnabled',
        value: String(Boolean(inputs.OkpayAutoExchangeEnabled)),
      });
      if (inputs.OkpayUsdtCnyRate !== '') {
        options.push({
          key: 'OkpayUsdtCnyRate',
          value: inputs.OkpayUsdtCnyRate.toString(),
        });
      }
      options.push({
        key: 'OkpayRateApiUrl',
        value: removeTrailingSlash(inputs.OkpayRateApiUrl || ''),
      });
      if (inputs.OkpayMinTopUp !== '') {
        options.push({
          key: 'OkpayMinTopUp',
          value: inputs.OkpayMinTopUp.toString(),
        });
      }
      options.push({ key: 'OkpayCoin', value: inputs.OkpayCoin || '' });

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
                field='OkpayGatewayUrl'
                label={t('网关地址')}
                placeholder='https://api.okaypay.me/shop'
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='OkpayMerchantId'
                label={t('商户 ID')}
                placeholder={t('例如：10001')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='OkpayMerchantToken'
                label={t('商户令牌')}
                placeholder={t('敏感信息不会发送到前端显示')}
                type='password'
              />
            </Col>
          </Row>
          <Row
            gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
            style={{ marginTop: 16 }}
          >
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.InputNumber
                field='OkpayExchangeRate'
                precision={2}
                label={t('站内充值单价（CNY/USD）')}
                placeholder={t('例如：7.2，表示 1 美元额度标价 7.2 元')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.InputNumber
                field='OkpayMinTopUp'
                label={t('最低充值美元数量')}
                placeholder={t('例如：2，就是最低充值2$')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='OkpayCoin'
                label={t('币种')}
                placeholder='USDT'
              />
            </Col>
          </Row>
          <Row
            gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
            style={{ marginTop: 16 }}
          >
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Switch
                field='OkpayAutoExchangeEnabled'
                label={t('自动获取 USDT/CNY 汇率')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.InputNumber
                field='OkpayUsdtCnyRate'
                precision={4}
                label={t('USDT/CNY 兜底汇率')}
                placeholder={t('自动汇率失败时使用')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='OkpayRateApiUrl'
                label={t('汇率接口地址')}
                placeholder='https://api.coingecko.com/api/v3/simple/price?ids=tether&vs_currencies=cny&include_last_updated_at=true'
              />
            </Col>
          </Row>
          <Button onClick={submitOkpaySetting} style={{ marginTop: 16 }}>
            {t('更新 OKPay 设置')}
          </Button>
        </Form.Section>
      </Form>
    </Spin>
  );
}
