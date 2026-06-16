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

export default function SettingsPaymentGatewayBepusdt(props) {
  const { t } = useTranslation();
  const sectionTitle = props.hideSectionTitle
    ? undefined
    : t('Bepusdt (USDT) 设置');
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    BepusdtApiUrl: '',
    BepusdtAuthToken: '',
    BepusdtUnitPrice: 7.3,
    BepusdtMinTopUp: 1,
    BepusdtTimeout: 600,
    BepusdtChains: '',
  });
  const formApiRef = useRef(null);

  useEffect(() => {
    if (props.options && formApiRef.current) {
      const currentInputs = {
        BepusdtApiUrl: props.options.BepusdtApiUrl || '',
        BepusdtAuthToken: props.options.BepusdtAuthToken || '',
        BepusdtUnitPrice:
          props.options.BepusdtUnitPrice !== undefined
            ? parseFloat(props.options.BepusdtUnitPrice)
            : 7.3,
        BepusdtMinTopUp:
          props.options.BepusdtMinTopUp !== undefined
            ? parseInt(props.options.BepusdtMinTopUp)
            : 1,
        BepusdtTimeout:
          props.options.BepusdtTimeout !== undefined
            ? parseInt(props.options.BepusdtTimeout)
            : 600,
        BepusdtChains: props.options.BepusdtChains || '',
      };

      setInputs(currentInputs);
      formApiRef.current.setValues(currentInputs);
    }
  }, [props.options]);

  const handleFormChange = (values) => {
    setInputs(values);
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

      if (inputs.BepusdtAuthToken !== undefined && inputs.BepusdtAuthToken !== '') {
        options.push({ key: 'BepusdtAuthToken', value: inputs.BepusdtAuthToken });
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
      options.push({ key: 'BepusdtChains', value: inputs.BepusdtChains || '' });

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
                placeholder={t('例如：600')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.TextArea
                field='BepusdtChains'
                label={t('链配置（JSON）')}
                placeholder='[{"name":"TRC20","trade_type":"usdt.trc20"}]'
                autosize={{ minRows: 2, maxRows: 4 }}
              />
            </Col>
          </Row>
          <Button onClick={submitBepusdtSetting} style={{ marginTop: 16 }}>
            {t('更新 Bepusdt 设置')}
          </Button>
        </Form.Section>
      </Form>
    </Spin>
  );
}
