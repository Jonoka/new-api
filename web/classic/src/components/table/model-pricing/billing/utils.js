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

export const BILLING_GUIDE_STORAGE_KEY =
  'classic_model_pricing_billing_guide_seen_v1';

export const DEFAULT_TOKEN_COUNTS = Object.freeze({
  input: 1024,
  output: 500,
  cacheRead: 155,
  cacheWrite: 116,
});

const isFiniteNumber = (value) => Number.isFinite(Number(value));

const toFiniteNumber = (value, fallback = 0) =>
  isFiniteNumber(value) ? Number(value) : fallback;

const getRatioValue = (value, fallback = null) =>
  value !== undefined && value !== null && value !== '' && isFiniteNumber(value)
    ? Number(value)
    : fallback;

export const hasSeenBillingGuide = (storage) => {
  try {
    return storage?.getItem(BILLING_GUIDE_STORAGE_KEY) === '1';
  } catch (_error) {
    return false;
  }
};

export const markBillingGuideSeen = (storage) => {
  try {
    if (!storage) return false;
    storage.setItem(BILLING_GUIDE_STORAGE_KEY, '1');
    return true;
  } catch (_error) {
    return false;
  }
};

export const getBillingGuideStorage = (windowObject) => {
  try {
    return windowObject?.localStorage;
  } catch (_error) {
    return undefined;
  }
};

export const getBillingGuideModels = (models = []) =>
  models.filter(
    (model) =>
      model?.quota_type === 0 &&
      model?.billing_mode !== 'tiered_expr' &&
      model?.model_ratio !== undefined &&
      model?.model_ratio !== null &&
      model?.model_ratio !== '' &&
      isFiniteNumber(model?.model_ratio),
  );

export const pickBillingGuideModel = (models = [], preferredModelName) => {
  const eligibleModels = getBillingGuideModels(models);
  if (eligibleModels.length === 0) return null;

  const preferredNames = [preferredModelName, 'gpt-5.5'].filter(Boolean);
  for (const modelName of preferredNames) {
    const matched = eligibleModels.find(
      (model) => model.model_name === modelName,
    );
    if (matched) return matched;
  }

  return eligibleModels[0];
};

export const getBillingGuideGroups = (model, groupRatio = {}) => {
  const groupNames = Array.isArray(model?.enable_groups)
    ? model.enable_groups.filter((group) => group && group !== 'auto')
    : [];

  const groups = groupNames
    .map((group) => ({
      value: group,
      ratio: getRatioValue(groupRatio[group]),
    }))
    .filter((group) => group.ratio !== null);

  if (groups.length > 0) return groups;

  return [{ value: 'default', ratio: 1, synthetic: true }];
};

export const pickBillingGuideGroup = (
  model,
  groupRatio = {},
  preferredGroup,
) => {
  const groups = getBillingGuideGroups(model, groupRatio);
  const preferred = groups.find((group) => group.value === preferredGroup);
  if (preferred) return preferred;

  return groups.reduce((best, group) =>
    group.ratio < best.ratio ? group : best,
  );
};

export const getBillingFactors = ({
  groupRatio = 1,
  priceRate = 1,
  usdExchangeRate = 1,
}) => {
  const normalizedExchangeRate = toFiniteNumber(usdExchangeRate, 1);
  const normalizedPriceRate = toFiniteNumber(priceRate, 1);
  const forexFactor =
    normalizedExchangeRate > 0
      ? normalizedPriceRate / normalizedExchangeRate
      : 1;
  const normalizedGroupRatio = toFiniteNumber(groupRatio, 1);

  return {
    forexFactor,
    groupFactor: normalizedGroupRatio,
    compositeFactor: forexFactor * normalizedGroupRatio,
  };
};

export const getBillingCurrency = ({
  currency = 'USD',
  usdExchangeRate = 1,
  customExchangeRate = 1,
  customCurrencySymbol = '¤',
}) => {
  if (currency === 'CNY') {
    return {
      currency: 'CNY',
      symbol: '¥',
      multiplier: toFiniteNumber(usdExchangeRate, 1),
    };
  }

  if (currency === 'CUSTOM') {
    return {
      currency: 'CUSTOM',
      symbol: customCurrencySymbol || '¤',
      multiplier: toFiniteNumber(customExchangeRate, 1),
    };
  }

  return { currency: 'USD', symbol: '$', multiplier: 1 };
};

export const parseBillingPrice = (value) => {
  if (typeof value === 'number') return Number.isFinite(value) ? value : null;
  if (typeof value !== 'string') return null;

  const normalized = value.replace(/,/gu, '');
  const matched = normalized.match(
    /[+-]?(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?/u,
  );
  if (!matched) return null;
  const parsed = Number(matched[0]);
  return Number.isFinite(parsed) ? parsed : null;
};

export const getBillingUnitPricesFromPriceData = ({
  priceData,
  currency = 'USD',
  usdExchangeRate = 1,
  customExchangeRate = 1,
  customCurrencySymbol = '¤',
}) => {
  if (!priceData?.isPerToken || priceData.isTokensDisplay) return null;

  const currencyMeta = getBillingCurrency({
    currency,
    usdExchangeRate,
    customExchangeRate,
    customCurrencySymbol,
  });
  const makePrice = (price, officialPrice) => {
    const unitPrice = parseBillingPrice(price);
    if (unitPrice === null) return null;
    const parsedOfficialPrice = parseBillingPrice(officialPrice);
    return {
      unitPrice,
      officialPrice:
        parsedOfficialPrice === null
          ? unitPrice
          : parsedOfficialPrice * currencyMeta.multiplier,
    };
  };

  return {
    ...currencyMeta,
    input: makePrice(priceData.inputPrice, priceData.originalInputPrice),
    output: makePrice(
      priceData.completionPrice,
      priceData.originalCompletionPrice,
    ),
    cacheRead: makePrice(priceData.cachePrice, priceData.originalCachePrice),
    cacheWrite: makePrice(
      priceData.createCachePrice,
      priceData.originalCreateCachePrice,
    ),
  };
};

export const calculateTokenCost = (tokens, unitPrice) => {
  const safeTokens = Math.max(0, toFiniteNumber(tokens));
  const safeUnitPrice = Math.max(0, toFiniteNumber(unitPrice));
  return (safeTokens / 1000000) * safeUnitPrice;
};

export const formatBillingNumber = (value, maximumFractionDigits = 4) => {
  const normalized = toFiniteNumber(value);
  return normalized.toLocaleString(undefined, {
    minimumFractionDigits: 0,
    maximumFractionDigits,
  });
};

export const formatBillingMoney = (symbol, value, digits = 6) =>
  `${symbol}${toFiniteNumber(value).toFixed(digits)}`;
