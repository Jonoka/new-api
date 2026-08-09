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

import React from 'react';
import { Banner, Input, Radio, TextArea, Typography } from '@douyinfe/semi-ui';

const { Text } = Typography;

export const createEmptyInvoiceRequest = (
  type = 'personal',
  kind = 'normal',
) => ({
  required: false,
  type,
  kind,
  title: '',
  tax_no: '',
  email: '',
  phone: '',
  remark: '',
});

const getTypeLabel = (type, t) => {
  if (type === 'company') return t('对公');
  if (type === 'personal') return t('对私');
  return type;
};

const getKindLabel = (kind, t) => {
  if (kind === 'normal') return t('增值税普通发票');
  if (kind === 'special') return t('增值税专用发票');
  return kind;
};

const InvoiceRequestForm = ({
  t,
  config,
  value,
  onChange,
  invoiceFee = 0,
  showRequiredToggle = true,
}) => {
  const invoice = value || createEmptyInvoiceRequest();
  const enabled = !!config?.enabled;
  const types =
    Array.isArray(config?.types) && config.types.length > 0
      ? config.types
      : ['personal', 'company'];
  const kinds =
    Array.isArray(config?.kinds) && config.kinds.length > 0
      ? config.kinds
      : ['normal'];
  const selectedType = types.includes(invoice.type) ? invoice.type : types[0];
  const selectedKind = kinds.includes(invoice.kind) ? invoice.kind : kinds[0];
  const patchInvoice = (patch) =>
    onChange?.({
      ...invoice,
      type: selectedType,
      kind: selectedKind,
      ...patch,
    });

  if (!enabled) {
    return null;
  }

  return (
    <div className='space-y-3'>
      {showRequiredToggle && (
        <div className='flex items-center justify-between'>
          <Text strong>{t('需要开发票')}</Text>
          <Radio.Group
            type='button'
            buttonSize='small'
            value={invoice.required ? 'yes' : 'no'}
            onChange={(event) =>
              patchInvoice({
                required: event.target.value === 'yes',
                type: selectedType,
                kind: selectedKind,
              })
            }
          >
            <Radio value='no'>{t('否')}</Radio>
            <Radio value='yes'>{t('是')}</Radio>
          </Radio.Group>
        </div>
      )}
      {invoice.required && (
        <>
          <Banner
            type='info'
            closeIcon={null}
            description={`${t('发票抬头类型')}：${types
              .map((type) => getTypeLabel(type, t))
              .join(' / ')}；${t('开票票种')}：${kinds
              .map((kind) => getKindLabel(kind, t))
              .join(' / ')}${
              Number(invoiceFee || 0) > 0
                ? `，${t('发票费用')}：¥${Number(invoiceFee).toFixed(2)}`
                : ''
            }`}
          />
          <div className='space-y-1'>
            <Text type='tertiary' size='small'>
              {t('发票抬头类型')}
            </Text>
            <Radio.Group
              type='button'
              buttonSize='small'
              value={selectedType}
              onChange={(event) => patchInvoice({ type: event.target.value })}
            >
              {types.map((type) => (
                <Radio key={type} value={type}>
                  {getTypeLabel(type, t)}
                </Radio>
              ))}
            </Radio.Group>
          </div>
          <div className='space-y-1'>
            <Text type='tertiary' size='small'>
              {t('开票票种')}
            </Text>
            <Radio.Group
              type='button'
              buttonSize='small'
              value={selectedKind}
              onChange={(event) => patchInvoice({ kind: event.target.value })}
            >
              {kinds.map((kind) => (
                <Radio key={kind} value={kind}>
                  {getKindLabel(kind, t)}
                </Radio>
              ))}
            </Radio.Group>
          </div>
          <Input
            value={invoice.title}
            onChange={(title) => patchInvoice({ title })}
            placeholder={t('发票抬头')}
          />
          <Input
            value={invoice.tax_no}
            onChange={(tax_no) => patchInvoice({ tax_no })}
            placeholder={t('纳税人识别号')}
          />
          <div className='grid grid-cols-1 sm:grid-cols-2 gap-2'>
            <Input
              value={invoice.email}
              onChange={(email) => patchInvoice({ email })}
              placeholder={t('接收邮箱')}
            />
            <Input
              value={invoice.phone}
              onChange={(phone) => patchInvoice({ phone })}
              placeholder={t('联系电话')}
            />
          </div>
          <TextArea
            value={invoice.remark}
            onChange={(remark) => patchInvoice({ remark })}
            placeholder={t('发票备注')}
            autosize
          />
        </>
      )}
    </div>
  );
};

export default InvoiceRequestForm;
