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
import {
  Banner,
  Modal,
  Typography,
  Card,
  Button,
  Divider,
  Tooltip,
  Input,
} from '@douyinfe/semi-ui';
import { Crown, CalendarClock, Package } from 'lucide-react';
import { SiStripe } from 'react-icons/si';
import { IconCreditCard } from '@douyinfe/semi-icons';
import { renderQuota } from '../../../helpers';
import {
  formatSubscriptionPlanAmount,
  formatSubscriptionDuration,
  formatSubscriptionResetPeriod,
} from '../../../helpers/subscriptionFormat';
import InvoiceRequestForm from '../../invoice/InvoiceRequestForm';

const { Text } = Typography;

const SubscriptionPurchaseModal = ({
  t,
  visible,
  onCancel,
  selectedPlan,
  paying,
  selectedEpayMethod,
  setSelectedEpayMethod,
  epayMethods = [],
  enableBepusdtTopUp = false,
  bepusdtChains = [],
  selectedBepusdtTradeType,
  setSelectedBepusdtTradeType,
  selectedPaymentKind = '',
  onSelectPaymentKind,
  enableOnlineTopUp = false,
  enableStripeTopUp = false,
  enableCreemTopUp = false,
  purchaseLimitInfo = null,
  promoCode,
  setPromoCode,
  promoDiscount,
  amountPreview,
  amountLoading = false,
  invoiceConfig,
  invoiceRequest,
  setInvoiceRequest,
  onPromoCodeBlur,
  onPayStripe,
  onPayCreem,
  onPayBepusdt,
  onPayEpay,
}) => {
  const plan = selectedPlan?.plan;
  const totalAmount = Number(plan?.total_amount || 0);
  const price = plan ? Number(plan.price_amount || 0) : 0;
  const hasPromoDiscount =
    promoDiscount && Number(promoDiscount.discount_amount || 0) > 0;
  const paidPrice = hasPromoDiscount
    ? Number(promoDiscount.paid_amount || 0)
    : price;
  const previewAmount =
    typeof amountPreview?.amount === 'number'
      ? amountPreview.amount
      : Number(amountPreview?.data || Number.NaN);
  const amountDue = Number.isFinite(previewAmount) ? previewAmount : paidPrice;
  const amountDueCurrency = amountPreview?.currency || plan?.currency || 'USD';
  const invoiceFee = Number(amountPreview?.invoice_fee || 0);
  const planPriceText = formatSubscriptionPlanAmount(price, plan?.currency);
  const displayPrice = formatSubscriptionPlanAmount(
    amountDue,
    amountDueCurrency,
  );
  const originalPrice = Number(promoDiscount?.original_amount || price);
  const discountAmount = Number(promoDiscount?.discount_amount || 0);
  const displayOriginalPrice = formatSubscriptionPlanAmount(
    originalPrice,
    amountDueCurrency,
  );
  const displayDiscountAmount = formatSubscriptionPlanAmount(
    discountAmount,
    amountDueCurrency,
  );
  // 只有当管理员开启支付网关 AND 套餐配置了对应的支付ID时才显示
  const hasStripe = enableStripeTopUp && !!plan?.stripe_price_id;
  const hasCreem = enableCreemTopUp && !!plan?.creem_product_id;
  const hasBepusdt =
    enableBepusdtTopUp &&
    Array.isArray(bepusdtChains) &&
    bepusdtChains.length > 0;
  const hasEpay = enableOnlineTopUp && epayMethods.length > 0;
  const hasAnyPayment = hasStripe || hasCreem || hasBepusdt || hasEpay;
  const paymentOptions = [
    ...(hasEpay
      ? epayMethods.map((method) => ({
          key: `epay:${method.type}`,
          kind: 'epay',
          value: method.type,
          label: method.name || method.type,
        }))
      : []),
    ...(hasBepusdt
      ? [{ key: 'bepusdt', kind: 'bepusdt', value: 'bepusdt', label: 'USDT' }]
      : []),
    ...(hasStripe
      ? [{ key: 'stripe', kind: 'stripe', value: 'stripe', label: 'Stripe' }]
      : []),
    ...(hasCreem
      ? [{ key: 'creem', kind: 'creem', value: 'creem', label: 'Creem' }]
      : []),
  ];
  const selectedEpayLabel =
    epayMethods.find((method) => method.type === selectedEpayMethod)?.name ||
    selectedEpayMethod;
  const purchaseLimit = Number(purchaseLimitInfo?.limit || 0);
  const purchaseCount = Number(purchaseLimitInfo?.count || 0);
  const purchaseLimitReached =
    purchaseLimit > 0 && purchaseCount >= purchaseLimit;

  return (
    <Modal
      title={
        <div className='flex items-center'>
          <Crown className='mr-2' size={18} />
          {t('购买订阅套餐')}
        </div>
      }
      visible={visible}
      onCancel={onCancel}
      footer={null}
      size='small'
      centered
    >
      {plan ? (
        <div className='space-y-4 pb-10'>
          {/* 套餐信息 */}
          <Card className='!rounded-xl !border-0 bg-slate-50 dark:bg-slate-800'>
            <div className='space-y-3'>
              <div className='flex justify-between items-center'>
                <Text strong className='text-slate-700 dark:text-slate-200'>
                  {t('套餐名称')}：
                </Text>
                <Typography.Text
                  ellipsis={{ rows: 1, showTooltip: true }}
                  className='text-slate-900 dark:text-slate-100'
                  style={{ maxWidth: 200 }}
                >
                  {plan.title}
                </Typography.Text>
              </div>
              <div className='flex justify-between items-center'>
                <Text strong className='text-slate-700 dark:text-slate-200'>
                  {t('有效期')}：
                </Text>
                <div className='flex items-center'>
                  <CalendarClock size={14} className='mr-1 text-slate-500' />
                  <Text className='text-slate-900 dark:text-slate-100'>
                    {formatSubscriptionDuration(plan, t)}
                  </Text>
                </div>
              </div>
              {formatSubscriptionResetPeriod(plan, t) !== t('不重置') && (
                <div className='flex justify-between items-center'>
                  <Text strong className='text-slate-700 dark:text-slate-200'>
                    {t('重置周期')}：
                  </Text>
                  <Text className='text-slate-900 dark:text-slate-100'>
                    {formatSubscriptionResetPeriod(plan, t)}
                  </Text>
                </div>
              )}
              <div className='flex justify-between items-center'>
                <Text strong className='text-slate-700 dark:text-slate-200'>
                  {t('总额度')}：
                </Text>
                <div className='flex items-center'>
                  <Package size={14} className='mr-1 text-slate-500' />
                  {totalAmount > 0 ? (
                    <Tooltip content={`${t('原生额度')}：${totalAmount}`}>
                      <Text className='text-slate-900 dark:text-slate-100'>
                        {renderQuota(totalAmount)}
                      </Text>
                    </Tooltip>
                  ) : (
                    <Text className='text-slate-900 dark:text-slate-100'>
                      {t('不限')}
                    </Text>
                  )}
                </div>
              </div>
              {plan?.upgrade_group ? (
                <div className='flex justify-between items-center'>
                  <Text strong className='text-slate-700 dark:text-slate-200'>
                    {t('升级分组')}：
                  </Text>
                  <Text className='text-slate-900 dark:text-slate-100'>
                    {plan.upgrade_group}
                  </Text>
                </div>
              ) : null}
              <Divider margin={8} />
              <div className='flex justify-between items-center'>
                <Text strong className='text-slate-700 dark:text-slate-200'>
                  {t('套餐价格')}：
                </Text>
                <Text className='text-slate-900 dark:text-slate-100'>
                  {planPriceText}
                </Text>
              </div>
              <div className='flex justify-between items-center'>
                <Text strong className='text-slate-700 dark:text-slate-200'>
                  {t('应付金额')}：
                </Text>
                {amountLoading ? (
                  <Text type='tertiary'>{t('计算中')}</Text>
                ) : (
                  <div className='flex items-baseline space-x-2'>
                    <Text strong className='text-xl text-purple-600'>
                      {displayPrice}
                    </Text>
                    {hasPromoDiscount && (
                      <Text size='small' className='text-rose-500'>
                        {t('已优惠')}
                      </Text>
                    )}
                  </div>
                )}
              </div>
              {hasPromoDiscount && !amountLoading && (
                <>
                  <div className='flex justify-between items-center'>
                    <Text className='text-slate-500 dark:text-slate-400'>
                      {t('原价')}：
                    </Text>
                    <Text delete className='text-slate-500 dark:text-slate-400'>
                      {displayOriginalPrice}
                    </Text>
                  </div>
                  <div className='flex justify-between items-center'>
                    <Text className='text-slate-500 dark:text-slate-400'>
                      {t('优惠')}：
                    </Text>
                    <Text className='text-emerald-600 dark:text-emerald-400'>
                      - {displayDiscountAmount}
                    </Text>
                  </div>
                </>
              )}
              <div className='flex justify-between items-center gap-3'>
                <Text strong className='text-slate-700 dark:text-slate-200'>
                  {t('优惠码')}：
                </Text>
                <Input
                  value={promoCode}
                  onChange={setPromoCode}
                  onBlur={onPromoCodeBlur}
                  placeholder={t('可选')}
                  size='small'
                  style={{ width: 180 }}
                />
              </div>
              <InvoiceRequestForm
                t={t}
                config={invoiceConfig}
                value={invoiceRequest}
                onChange={setInvoiceRequest}
                invoiceFee={invoiceFee}
              />
            </div>
          </Card>

          {/* 支付方式 */}
          {purchaseLimitReached && (
            <Banner
              type='warning'
              description={`${t('已达到购买上限')} (${purchaseCount}/${purchaseLimit})`}
              className='!rounded-xl'
              closeIcon={null}
            />
          )}

          {hasAnyPayment ? (
            <div className='space-y-3'>
              <Text size='small' type='tertiary'>
                {t('选择支付方式')}：
              </Text>

              <div className='grid grid-cols-2 sm:grid-cols-3 gap-2'>
                {paymentOptions.map((option) => {
                  const selected =
                    option.kind === 'epay'
                      ? selectedPaymentKind === 'epay' &&
                        selectedEpayMethod === option.value
                      : selectedPaymentKind === option.kind;
                  return (
                    <Button
                      key={option.key}
                      theme={selected ? 'solid' : 'light'}
                      type='primary'
                      onClick={() =>
                        onSelectPaymentKind?.(option.kind, option.value)
                      }
                      disabled={purchaseLimitReached || amountLoading}
                      icon={
                        option.kind === 'stripe' ? (
                          <SiStripe size={14} color={selected ? '#fff' : '#635BFF'} />
                        ) : option.kind === 'creem' ? (
                          <IconCreditCard />
                        ) : null
                      }
                    >
                      {option.label}
                    </Button>
                  );
                })}
              </div>

              {selectedPaymentKind === 'bepusdt' && hasBepusdt && (
                <div className='space-y-2'>
                  <div className='grid grid-cols-2 gap-2'>
                    {bepusdtChains.map((chain) => (
                      <Button
                        key={chain.trade_type}
                        theme={
                          selectedBepusdtTradeType === chain.trade_type
                            ? 'solid'
                            : 'light'
                        }
                        type='primary'
                        onClick={() => setSelectedBepusdtTradeType(chain.trade_type)}
                        disabled={purchaseLimitReached}
                      >
                        {chain.name || chain.trade_type}
                      </Button>
                    ))}
                  </div>
                  <Banner
                    type='info'
                    description={t('USDT 订阅支付无平台手续费')}
                    className='!rounded-xl'
                    closeIcon={null}
                  />
                  <Button
                    theme='solid'
                    type='primary'
                    block
                    onClick={onPayBepusdt}
                    loading={paying}
                    disabled={!selectedBepusdtTradeType || purchaseLimitReached}
                  >
                    USDT
                  </Button>
                </div>
              )}

              {selectedPaymentKind === 'epay' && hasEpay && (
                <div className='flex gap-2 items-center'>
                  <div className='flex-1 rounded-lg border border-solid border-[var(--semi-color-border)] px-3 py-2 text-sm'>
                    {selectedEpayLabel || t('选择支付方式')}
                  </div>
                  <Button
                    theme='solid'
                    type='primary'
                    onClick={onPayEpay}
                    loading={paying}
                    disabled={!selectedEpayMethod || purchaseLimitReached}
                  >
                    {t('支付')}
                  </Button>
                </div>
              )}
              {selectedPaymentKind === 'stripe' && hasStripe && (
                <Button
                  theme='solid'
                  type='primary'
                  block
                  icon={<SiStripe size={14} color='#fff' />}
                  onClick={onPayStripe}
                  loading={paying}
                  disabled={purchaseLimitReached}
                >
                  Stripe
                </Button>
              )}
              {selectedPaymentKind === 'creem' && hasCreem && (
                <Button
                  theme='solid'
                  type='primary'
                  block
                  icon={<IconCreditCard />}
                  onClick={onPayCreem}
                  loading={paying}
                  disabled={purchaseLimitReached}
                >
                  Creem
                </Button>
              )}
            </div>
          ) : (
            <Banner
              type='info'
              description={t('管理员未开启在线支付功能，请联系管理员配置。')}
              className='!rounded-xl'
              closeIcon={null}
            />
          )}
        </div>
      ) : null}
    </Modal>
  );
};

export default SubscriptionPurchaseModal;
