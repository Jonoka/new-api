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

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import {
  Button,
  Divider,
  InputNumber,
  Modal,
  Select,
  Tag,
} from '@douyinfe/semi-ui';
import {
  IconArrowLeft,
  IconArrowRight,
  IconHelpCircle,
  IconRefresh,
} from '@douyinfe/semi-icons';
import { calculateModelPrice } from '../../../../helpers';
import { BILLING_GUIDE_MASK_STYLE } from './BillingGuideWelcome';
import {
  DEFAULT_TOKEN_COUNTS,
  calculateTokenCost,
  formatBillingMoney,
  formatBillingNumber,
  getBillingDiscountColor,
  getBillingDiscountText,
  getBillingFactors,
  getBillingGuideGroups,
  getBillingGuideModels,
  getBillingUnitPricesFromPriceData,
  pickBillingGuideGroup,
  pickBillingGuideModel,
} from './utils';

const STEP_COUNT = 2;

const CARD_STYLE = {
  border: '1px solid var(--semi-color-border)',
  borderRadius: 12,
  backgroundColor: 'var(--semi-color-bg-0)',
};

const ACTIVE_CARD_STYLE = {
  ...CARD_STYLE,
  borderColor: 'var(--semi-color-primary)',
  backgroundColor: 'var(--semi-color-primary-light-default)',
  boxShadow: '0 0 0 1px var(--semi-color-primary-light-active)',
};

const stripTrailingZeros = (value) =>
  String(value)
    .replace(/(\.\d*?[1-9])0+$/u, '$1')
    .replace(/\.0+$/u, '');

const formatFixedNumber = (value, digits = 4) =>
  stripTrailingZeros(Number(value || 0).toFixed(digits));

const formatUnitPrice = (symbol, value) =>
  `${symbol}${formatFixedNumber(value, 4)}/M`;

const FormulaItem = ({ index, title, formula, active, children }) => (
  <div className='p-3' style={active ? ACTIVE_CARD_STYLE : CARD_STYLE}>
    <div className='flex items-center justify-between gap-3'>
      <div className='flex min-w-0 items-center gap-2'>
        <span
          className='font-mono text-xs'
          style={{ color: 'var(--semi-color-text-3)' }}
        >
          {String(index).padStart(2, '0')}
        </span>
        <span
          className='font-semibold'
          style={{ color: 'var(--semi-color-text-0)' }}
        >
          {title}
        </span>
      </div>
      <span
        className='shrink-0 font-mono text-sm font-semibold'
        style={{ color: 'var(--semi-color-text-0)' }}
      >
        {formula}
      </span>
    </div>
    {children && (
      <div
        className='mt-2 text-xs leading-5'
        style={{ color: 'var(--semi-color-text-2)' }}
      >
        {children}
      </div>
    )}
  </div>
);

const PriceLine = ({ label, price, officialPrice, symbol }) => {
  if (!price) return null;
  const hasDiscount = Math.abs(price.unitPrice - officialPrice) > 0.0000001;

  return (
    <div className='flex flex-wrap items-baseline gap-x-2 gap-y-1 text-sm'>
      <span style={{ color: 'var(--semi-color-text-2)' }}>{label}</span>
      <span className='font-mono font-medium'>
        {formatUnitPrice(symbol, price.unitPrice)}
      </span>
      {hasDiscount && (
        <span
          className='font-mono text-xs line-through'
          style={{ color: 'var(--semi-color-text-3)' }}
        >
          {formatUnitPrice(symbol, officialPrice)}
        </span>
      )}
    </div>
  );
};

const ModelGroupSelector = ({
  isMobile,
  modelOptions,
  groupOptions,
  selectedModelName,
  selectedGroupName,
  onModelChange,
  onGroupChange,
  t,
}) => (
  <div className={`flex gap-2 ${isMobile ? 'flex-col' : 'items-center'}`}>
    <div className='flex min-w-0 flex-1 items-center gap-2'>
      <span
        id='billing-guide-model-label'
        className='shrink-0 text-xs'
        style={{ color: 'var(--semi-color-text-2)' }}
      >
        {t('模型')}
      </span>
      <Select
        className='min-w-0 flex-1'
        aria-labelledby='billing-guide-model-label'
        value={selectedModelName}
        optionList={modelOptions}
        onChange={onModelChange}
        filter
        placeholder={t('选择模型')}
      />
    </div>
    <div className='flex min-w-0 flex-1 items-center gap-2'>
      <span
        id='billing-guide-group-label'
        className='shrink-0 text-xs'
        style={{ color: 'var(--semi-color-text-2)' }}
      >
        {t('分组')}
      </span>
      <Select
        className='min-w-0 flex-1'
        aria-labelledby='billing-guide-group-label'
        value={selectedGroupName}
        optionList={groupOptions}
        onChange={onGroupChange}
        placeholder={t('选择分组')}
      />
    </div>
  </div>
);

const BillingGuide = ({
  visible,
  onClose,
  isMobile,
  models = [],
  groupRatio = {},
  selectedGroup = 'all',
  currency = 'USD',
  siteDisplayType = 'USD',
  priceRate = 1,
  usdExchangeRate = 1,
  customExchangeRate = 1,
  customCurrencySymbol = '¤',
  displayPrice,
  t,
}) => {
  const [step, setStep] = useState(0);
  const [selectedModelName, setSelectedModelName] = useState('');
  const [selectedGroupName, setSelectedGroupName] = useState('');
  const [tokenCounts, setTokenCounts] = useState(DEFAULT_TOKEN_COUNTS);
  const [animatedTotal, setAnimatedTotal] = useState(0);
  const [animationKey, setAnimationKey] = useState(0);
  const initializedForOpenRef = useRef(false);
  const scrollContainerRef = useRef(null);

  const guideModels = useMemo(() => getBillingGuideModels(models), [models]);
  const selectedModel = useMemo(
    () =>
      guideModels.find((model) => model.model_name === selectedModelName) ||
      null,
    [guideModels, selectedModelName],
  );
  const groups = useMemo(
    () => getBillingGuideGroups(selectedModel, groupRatio),
    [selectedModel, groupRatio],
  );
  const selectedGroupInfo = useMemo(
    () =>
      groups.find((group) => group.value === selectedGroupName) || groups[0],
    [groups, selectedGroupName],
  );

  const effectiveCurrency = siteDisplayType === 'TOKENS' ? 'USD' : currency;
  const priceData = useMemo(
    () =>
      selectedModel && typeof displayPrice === 'function'
        ? calculateModelPrice({
            record: selectedModel,
            selectedGroup: selectedGroupName,
            groupRatio,
            tokenUnit: 'M',
            displayPrice,
            currency: effectiveCurrency,
            quotaDisplayType:
              siteDisplayType === 'TOKENS' ? 'USD' : siteDisplayType,
            precision: 6,
          })
        : null,
    [
      selectedModel,
      selectedGroupName,
      groupRatio,
      displayPrice,
      effectiveCurrency,
      siteDisplayType,
    ],
  );
  const prices = useMemo(
    () =>
      getBillingUnitPricesFromPriceData({
        priceData,
        currency: effectiveCurrency,
        usdExchangeRate,
        customExchangeRate,
        customCurrencySymbol,
      }),
    [
      priceData,
      effectiveCurrency,
      usdExchangeRate,
      customExchangeRate,
      customCurrencySymbol,
    ],
  );

  const factors = useMemo(
    () =>
      getBillingFactors({
        groupRatio: selectedGroupInfo?.ratio ?? 1,
        priceRate,
        usdExchangeRate,
      }),
    [selectedGroupInfo, priceRate, usdExchangeRate],
  );

  const modelOptions = useMemo(
    () =>
      guideModels.map((model) => ({
        value: model.model_name,
        label: model.model_name,
      })),
    [guideModels],
  );
  const groupOptions = useMemo(
    () =>
      groups.map((group) => ({
        value: group.value,
        label: `${group.value} · ×${formatFixedNumber(group.ratio, 3)}`,
      })),
    [groups],
  );

  useEffect(() => {
    if (!visible) {
      initializedForOpenRef.current = false;
      return;
    }
    if (initializedForOpenRef.current || guideModels.length === 0) return;

    const model = pickBillingGuideModel(guideModels);
    const group = pickBillingGuideGroup(model, groupRatio, selectedGroup);
    setSelectedModelName(model?.model_name || '');
    setSelectedGroupName(group?.value || '');
    setTokenCounts(DEFAULT_TOKEN_COUNTS);
    setStep(0);
    setAnimationKey((value) => value + 1);
    initializedForOpenRef.current = true;
  }, [visible, guideModels, groupRatio, selectedGroup]);

  useEffect(() => {
    if (!visible) return undefined;
    const frameId = requestAnimationFrame(() => {
      scrollContainerRef.current?.scrollTo({ top: 0 });
    });
    return () => cancelAnimationFrame(frameId);
  }, [visible, step]);

  const handleModelChange = useCallback(
    (modelName) => {
      const model = guideModels.find((item) => item.model_name === modelName);
      const group = pickBillingGuideGroup(model, groupRatio, selectedGroupName);
      setSelectedModelName(modelName);
      setSelectedGroupName(group?.value || '');
    },
    [guideModels, groupRatio, selectedGroupName],
  );

  const costRows = useMemo(() => {
    if (!prices) return [];
    return [
      {
        key: 'input',
        label: t('输入'),
        description: t('未命中缓存的 prompt token'),
        price: prices.input,
      },
      {
        key: 'output',
        label: t('输出'),
        description: t('模型生成的 completion token'),
        price: prices.output,
      },
      {
        key: 'cacheRead',
        label: t('缓存读取'),
        description: t('命中缓存的 prompt token'),
        price: prices.cacheRead,
        badge: t('读'),
      },
      {
        key: 'cacheWrite',
        label: t('缓存创建'),
        description: t('本次写入缓存的 token'),
        price: prices.cacheWrite,
        badge: t('写'),
      },
    ]
      .filter((row) => row.price)
      .map((row) => ({
        ...row,
        cost: calculateTokenCost(tokenCounts[row.key], row.price.unitPrice),
      }));
  }, [prices, tokenCounts, t]);

  const totalCost = useMemo(
    () => costRows.reduce((total, row) => total + row.cost, 0),
    [costRows],
  );

  useEffect(() => {
    if (!visible || step !== 1) return undefined;
    if (
      typeof window !== 'undefined' &&
      window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
    ) {
      setAnimatedTotal(totalCost);
      return undefined;
    }

    let frameId;
    const startedAt = performance.now();
    const duration = 700;
    const animate = (now) => {
      const progress = Math.min(1, (now - startedAt) / duration);
      const easedProgress = 1 - Math.pow(1 - progress, 3);
      setAnimatedTotal(totalCost * easedProgress);
      if (progress < 1) frameId = requestAnimationFrame(animate);
    };
    setAnimatedTotal(0);
    frameId = requestAnimationFrame(animate);

    return () => cancelAnimationFrame(frameId);
  }, [visible, step, totalCost, animationKey]);

  const handleTokenChange = useCallback((key, value) => {
    const normalizedValue = Math.max(0, Math.floor(Number(value) || 0));
    setTokenCounts((current) => ({ ...current, [key]: normalizedValue }));
  }, []);

  const handleReplay = useCallback(() => {
    setTokenCounts(DEFAULT_TOKEN_COUNTS);
    setAnimationKey((value) => value + 1);
  }, []);

  const renderSelector = () => (
    <ModelGroupSelector
      isMobile={isMobile}
      modelOptions={modelOptions}
      groupOptions={groupOptions}
      selectedModelName={selectedModelName}
      selectedGroupName={selectedGroupName}
      onModelChange={handleModelChange}
      onGroupChange={setSelectedGroupName}
      t={t}
    />
  );

  const renderStepOne = () => (
    <div>
      <div
        className='mb-4 text-sm'
        style={{ color: 'var(--semi-color-text-2)' }}
      >
        {t('左侧先拆公式，右侧用同一个节奏带入一个示例')}
      </div>

      <div className={`flex gap-6 ${isMobile ? 'flex-col' : 'items-start'}`}>
        <section className={isMobile ? 'w-full' : 'w-1/2'}>
          <Divider align='left' margin='12px'>
            {t('公式构成')}
          </Divider>
          <div className='flex flex-col gap-2'>
            <FormulaItem
              index={1}
              title={t('汇率优惠')}
              formula={t('{{priceRate}} ÷ {{exchangeRate}}', {
                priceRate: formatFixedNumber(priceRate, 3),
                exchangeRate: formatFixedNumber(usdExchangeRate, 3),
              })}
            >
              <div>
                {t('本平台充值：1 美元额度约需 {{amount}} 元人民币', {
                  amount: formatFixedNumber(priceRate, 3),
                })}
              </div>
              <div className='mt-2 grid grid-cols-2 gap-2'>
                <div
                  className='rounded-lg p-2'
                  style={{ backgroundColor: 'var(--semi-color-fill-0)' }}
                >
                  <div>{t('美元计价模型')}</div>
                  <strong className='font-mono'>
                    × {formatBillingNumber(factors.forexFactor, 3)}
                  </strong>
                </div>
                <div
                  className='rounded-lg p-2'
                  style={{ backgroundColor: 'var(--semi-color-fill-0)' }}
                >
                  <div>{t('人民币计价模型')}</div>
                  <strong className='font-mono'>× 1</strong>
                </div>
              </div>
            </FormulaItem>
            <FormulaItem
              index={2}
              title={t('分组倍率')}
              formula={`× ${formatBillingNumber(factors.groupFactor, 3)}`}
              active
            >
              {t(
                '模型卡片上的各类单价，是官网价按当前分组倍率换算后的展示价格。',
              )}
            </FormulaItem>
            <FormulaItem
              index={3}
              title={t('综合折扣')}
              formula={t('汇率优惠 × 倍率')}
            >
              {t('相对厂商官方报价的综合折扣率。')}
            </FormulaItem>
          </div>
        </section>

        <section className={isMobile ? 'w-full' : 'w-1/2'}>
          <Divider align='left' margin='12px'>
            {t('卡片样例')}
          </Divider>
          {renderSelector()}

          <div className='mt-3 p-4' style={ACTIVE_CARD_STYLE}>
            {selectedModel && prices ? (
              <>
                <div className='mb-3 flex items-center justify-between gap-3'>
                  <strong
                    className='min-w-0 truncate text-base'
                    style={{ color: 'var(--semi-color-text-0)' }}
                  >
                    {selectedModel.model_name}
                  </strong>
                  <Tag
                    color={getBillingDiscountColor(factors.compositeFactor)}
                    shape='circle'
                  >
                    {getBillingDiscountText(factors.compositeFactor, t)}
                  </Tag>
                </div>
                <div className='flex flex-col gap-1.5'>
                  <PriceLine
                    label={t('输入')}
                    price={prices.input}
                    officialPrice={prices.input.officialPrice}
                    symbol={prices.symbol}
                  />
                  <PriceLine
                    label={t('输出')}
                    price={prices.output}
                    officialPrice={prices.output.officialPrice}
                    symbol={prices.symbol}
                  />
                  <PriceLine
                    label={t('缓存读取')}
                    price={prices.cacheRead}
                    officialPrice={prices.cacheRead?.officialPrice}
                    symbol={prices.symbol}
                  />
                  <PriceLine
                    label={t('缓存创建')}
                    price={prices.cacheWrite}
                    officialPrice={prices.cacheWrite?.officialPrice}
                    symbol={prices.symbol}
                  />
                </div>
                <div className='mt-4'>
                  <Tag color='purple' shape='circle'>
                    {t('按量计费')}
                  </Tag>
                </div>
              </>
            ) : (
              <div style={{ color: 'var(--semi-color-text-2)' }}>
                {t('暂无可演示的按量计费模型')}
              </div>
            )}
          </div>

          <Divider align='left' margin='16px'>
            {t('折扣拆分')}
          </Divider>
          <div className='p-3' style={CARD_STYLE}>
            <div className='flex items-center justify-between py-2 text-sm'>
              <span>{t('汇率优惠')}</span>
              <strong className='font-mono'>
                × {formatBillingNumber(factors.forexFactor, 3)}
              </strong>
            </div>
            <div
              className='flex items-center justify-between rounded-lg px-2 py-3 text-sm'
              style={ACTIVE_CARD_STYLE}
            >
              <span>{t('分组倍率')}</span>
              <strong className='font-mono'>
                × {formatBillingNumber(factors.groupFactor, 3)}
              </strong>
            </div>
            <Divider margin='10px' />
            <div className='flex items-center justify-between text-sm'>
              <span>{t('综合折扣')}</span>
              <strong style={{ color: 'var(--semi-color-primary)' }}>
                {formatBillingNumber(factors.compositeFactor, 3)} →{' '}
                {getBillingDiscountText(factors.compositeFactor, t)}
              </strong>
            </div>
          </div>
        </section>
      </div>
    </div>
  );

  const renderStepTwo = () => (
    <div>
      <div
        className='mb-4 text-sm'
        style={{ color: 'var(--semi-color-text-2)' }}
      >
        {t(
          '选一个模型 → 调整本次 token 数 → 看动画把每一类 token 的扣费累加成总价。',
        )}
      </div>

      <div
        className={`p-3 ${isMobile ? '' : 'flex items-center gap-3'}`}
        style={ACTIVE_CARD_STYLE}
      >
        <div className={isMobile ? 'w-full' : 'min-w-0 flex-1'}>
          {renderSelector()}
        </div>
        <Tag
          className={isMobile ? 'mt-3' : ''}
          color={getBillingDiscountColor(factors.compositeFactor)}
          shape='circle'
        >
          {getBillingDiscountText(factors.compositeFactor, t)}
        </Tag>
        <div
          className={`${isMobile ? 'mt-3 text-left' : 'min-w-[150px] text-right'}`}
        >
          <div
            className='text-xs'
            style={{ color: 'var(--semi-color-text-2)' }}
          >
            {t('实时合计')}
          </div>
          <div
            className='font-mono text-2xl font-bold'
            style={{ color: 'var(--semi-color-text-0)' }}
          >
            {formatBillingMoney(prices?.symbol || '$', animatedTotal, 6)}
          </div>
        </div>
      </div>

      {prices && (
        <div
          className='my-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs'
          style={{ color: 'var(--semi-color-text-2)' }}
        >
          <strong>{t('卡片单价')}</strong>
          {costRows.map((row) => (
            <span key={row.key}>
              {row.label}{' '}
              <strong className='font-mono'>
                {formatUnitPrice(prices.symbol, row.price.unitPrice)}
              </strong>
            </span>
          ))}
        </div>
      )}

      <div className='flex flex-col gap-2.5'>
        {costRows.map((row) => (
          <div key={row.key} className='p-3' style={CARD_STYLE}>
            <div className='flex items-start justify-between gap-3'>
              <div className='min-w-0'>
                <div className='flex items-center gap-2'>
                  {row.badge && (
                    <Tag
                      className='shrink-0'
                      color='green'
                      shape='circle'
                      size='small'
                    >
                      {row.badge}
                    </Tag>
                  )}
                  <strong>{row.label}</strong>
                  <span
                    className='text-xs'
                    style={{ color: 'var(--semi-color-text-3)' }}
                  >
                    {row.description}
                  </span>
                </div>
              </div>
              <strong
                className='shrink-0 font-mono text-sm'
                style={{ color: 'var(--semi-color-success)' }}
              >
                +{formatBillingMoney(prices.symbol, row.cost, 6)}
              </strong>
            </div>
            <div
              className={`mt-2 flex gap-2 ${isMobile ? 'flex-col' : 'items-center'}`}
            >
              <InputNumber
                aria-label={row.label}
                value={tokenCounts[row.key]}
                min={0}
                max={1000000000}
                precision={0}
                onChange={(value) => handleTokenChange(row.key, value)}
                style={{ width: isMobile ? '100%' : 150 }}
              />
              <span
                className='font-mono text-xs'
                style={{ color: 'var(--semi-color-text-2)' }}
              >
                ÷ 1,000,000 × {formatBillingNumber(row.price.unitPrice, 6)}
              </span>
            </div>
          </div>
        ))}
      </div>

      <div
        className='mt-3 flex flex-wrap items-center justify-between gap-3 text-xs'
        style={{ color: 'var(--semi-color-text-3)' }}
      >
        <span>
          {prices?.cacheWrite
            ? t(
                'Claude 缓存创建按 5m / 1h 分档计费，此处按模型卡片单价估算，实际账单可能有细微差异。',
              )
            : t('计算结果按模型卡片当前单价估算。')}
        </span>
        <Button
          theme='borderless'
          type='primary'
          icon={<IconRefresh />}
          onClick={handleReplay}
        >
          {t('重新播放')}
        </Button>
      </div>
    </div>
  );

  return (
    <Modal
      visible={visible}
      onCancel={onClose}
      title={
        <div className='flex items-center gap-2'>
          <IconHelpCircle />
          <span>{step === 0 ? t('计费说明') : t('实际花费计算')}</span>
        </div>
      }
      footer={null}
      width={isMobile ? '96%' : 860}
      maskStyle={BILLING_GUIDE_MASK_STYLE}
      bodyStyle={{ overflow: 'hidden', padding: 0 }}
      style={{ maxWidth: 'calc(100vw - 16px)' }}
    >
      <div
        ref={scrollContainerRef}
        style={{
          maxHeight: isMobile ? '76vh' : '72vh',
          overflowY: 'auto',
          padding: isMobile ? '12px 16px 14px' : '16px 24px 18px',
        }}
      >
        {step === 0 ? renderStepOne() : renderStepTwo()}

        <Divider margin='18px' />
        <div className='flex items-center justify-between gap-3'>
          <div className='min-w-[88px]'>
            {step > 0 && (
              <Button
                theme='borderless'
                type='tertiary'
                icon={<IconArrowLeft />}
                onClick={() => setStep((current) => Math.max(0, current - 1))}
              >
                {t('上一步')}
              </Button>
            )}
          </div>

          <div className='flex items-center gap-1.5'>
            {Array.from({ length: STEP_COUNT }, (_, index) => (
              <button
                key={index}
                type='button'
                aria-label={t('第 {{step}} 步', { step: index + 1 })}
                onClick={() => setStep(index)}
                className='h-2 rounded-full border-0 p-0 transition-all'
                style={{
                  width: index === step ? 18 : 8,
                  backgroundColor:
                    index === step
                      ? 'var(--semi-color-primary)'
                      : 'var(--semi-color-fill-1)',
                  cursor: 'pointer',
                }}
              />
            ))}
          </div>

          <div className='flex min-w-[88px] justify-end'>
            {step < STEP_COUNT - 1 ? (
              <Button
                theme='borderless'
                type='primary'
                icon={<IconArrowRight />}
                iconPosition='right'
                onClick={() => setStep((current) => current + 1)}
              >
                {t('下一步')}
              </Button>
            ) : (
              <Button theme='solid' type='primary' onClick={onClose}>
                {t('知道了')}
              </Button>
            )}
          </div>
        </div>
      </div>
    </Modal>
  );
};

export default BillingGuide;
