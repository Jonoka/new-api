import React from 'react';
import { Banner, Input, Radio, TextArea, Typography } from '@douyinfe/semi-ui';

const { Text } = Typography;

export const createEmptyInvoiceRequest = () => ({
  required: false,
  type: 'personal',
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

const InvoiceRequestForm = ({
  t,
  config,
  value,
  onChange,
  invoiceFee = 0,
}) => {
  const invoice = value || createEmptyInvoiceRequest();
  const enabled = !!config?.enabled;
  const types = Array.isArray(config?.types) && config.types.length > 0
    ? config.types
    : ['personal', 'company'];
  const patchInvoice = (patch) => onChange?.({ ...invoice, ...patch });

  if (!enabled) {
    return null;
  }

  return (
    <div className='space-y-3'>
      <div className='flex items-center justify-between'>
        <Text strong>{t('需要开发票')}</Text>
        <Radio.Group
          type='button'
          buttonSize='small'
          value={invoice.required ? 'yes' : 'no'}
          onChange={(event) =>
            patchInvoice({ required: event.target.value === 'yes' })
          }
        >
          <Radio value='no'>{t('否')}</Radio>
          <Radio value='yes'>{t('是')}</Radio>
        </Radio.Group>
      </div>
      {invoice.required && (
        <>
          <Banner
            type='info'
            closeIcon={null}
            description={`${t('支持开发票类型')}：${types
              .map((type) => getTypeLabel(type, t))
              .join(' / ')}${
              Number(invoiceFee || 0) > 0
                ? `，${t('发票费用')}：¥${Number(invoiceFee).toFixed(2)}`
                : ''
            }`}
          />
          <Radio.Group
            type='button'
            buttonSize='small'
            value={invoice.type || types[0]}
            onChange={(event) => patchInvoice({ type: event.target.value })}
          >
            {types.map((type) => (
              <Radio key={type} value={type}>
                {getTypeLabel(type, t)}
              </Radio>
            ))}
          </Radio.Group>
          <Input
            value={invoice.title}
            onChange={(title) => patchInvoice({ title })}
            placeholder={t('发票抬头')}
          />
          {(invoice.type || types[0]) === 'company' && (
            <Input
              value={invoice.tax_no}
              onChange={(tax_no) => patchInvoice({ tax_no })}
              placeholder={t('纳税人识别号')}
            />
          )}
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
