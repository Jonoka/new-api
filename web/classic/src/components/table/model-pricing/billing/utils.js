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

const BILLING_EXPRESSION_VARIABLES = [
  'p',
  'c',
  'cr',
  'cc',
  'cc1h',
  'img',
  'img_o',
  'ai',
  'ao',
];
const BILLING_NUMBER_PATTERN =
  '[+-]?(?:\\d+\\.?\\d*|\\.\\d+)(?:[eE][+-]?\\d+)?';
const BILLING_EXPRESSION_VARIABLE_PATTERN =
  BILLING_EXPRESSION_VARIABLES.join('|');
const BILLING_GUIDE_PRICE_VARIABLES = ['p', 'c', 'cr', 'cc'];
const BILLING_EXPRESSION_COEFFICIENT_REGEX = new RegExp(
  `\\b(${BILLING_EXPRESSION_VARIABLE_PATTERN})\\s*\\*\\s*(${BILLING_NUMBER_PATTERN})`,
  'g',
);
const BILLING_EXPRESSION_CONDITION_REGEX = new RegExp(
  `^(p|c|len)\\s*(<=|<|>=|>)\\s*(${BILLING_NUMBER_PATTERN})$`,
);
const BILLING_EXPRESSION_UNSUPPORTED_FUNCTION_REGEX =
  /\b(?:max|min|ceil|floor|abs|header|param|has|hour|minute|weekday|month|day)\s*\(/u;

const splitBillingExpressionArguments = (value) => {
  let depth = 0;
  let quote = '';
  let escaped = false;
  for (let index = 0; index < value.length; index += 1) {
    const char = value[index];
    if (quote) {
      if (escaped) {
        escaped = false;
      } else if (char === '\\') {
        escaped = true;
      } else if (char === quote) {
        quote = '';
      }
      continue;
    }
    if (char === '"' || char === "'") {
      quote = char;
    } else if (char === '(') {
      depth += 1;
    } else if (char === ')') {
      depth -= 1;
    } else if (char === ',' && depth === 0) {
      return [value.slice(0, index), value.slice(index + 1)];
    }
  }
  return [value, ''];
};

const parseBillingExpressionConditions = (value) => {
  const normalized = value
    .trim()
    .replace(/^\((.*)\)$/u, '$1')
    .trim();
  if (!normalized) return [];

  const conditions = normalized.split(/\s*&&\s*/u).map((part) => {
    const match = part.trim().match(BILLING_EXPRESSION_CONDITION_REGEX);
    if (!match) return null;
    return { var: match[1], op: match[2], value: Number(match[3]) };
  });
  return conditions.every(Boolean) ? conditions : null;
};

const parseBillingExpressionTierPrices = (value) => {
  const prices = Object.fromEntries(
    BILLING_EXPRESSION_VARIABLES.map((variable) => [variable, 0]),
  );
  const regex = new RegExp(BILLING_EXPRESSION_COEFFICIENT_REGEX.source, 'g');
  let match;
  while ((match = regex.exec(value)) !== null) {
    if (!(match[1] in prices)) continue;
    prices[match[1]] = Number(match[2]);
  }
  return prices;
};

const parseBillingExpression = (expression) => {
  const raw = String(expression || '').trim();
  if (!raw) return null;

  const ruleIndex = raw.indexOf('|||');
  const body = raw
    .slice(0, ruleIndex >= 0 ? ruleIndex : raw.length)
    .replace(/^v\d+:/u, '')
    .trim();
  if (!body) return null;

  const tiers = [];
  let supported = ruleIndex < 0 || !raw.slice(ruleIndex + 3).trim();
  let cursor = 0;
  let previousEnd = 0;

  while (cursor < body.length) {
    const tierStart = body.indexOf('tier(', cursor);
    if (tierStart < 0) break;

    let depth = 1;
    let quote = '';
    let escaped = false;
    let end = tierStart + 5;
    for (; end < body.length && depth > 0; end += 1) {
      const char = body[end];
      if (quote) {
        if (escaped) {
          escaped = false;
        } else if (char === '\\') {
          escaped = true;
        } else if (char === quote) {
          quote = '';
        }
        continue;
      }
      if (char === '"' || char === "'") {
        quote = char;
      } else if (char === '(') {
        depth += 1;
      } else if (char === ')') {
        depth -= 1;
      }
    }
    if (depth !== 0) {
      supported = false;
      break;
    }

    const [labelPart, valuePart] = splitBillingExpressionArguments(
      body.slice(tierStart + 5, end - 1),
    );
    const labelMatch = labelPart.trim().match(/^(?:"([^"]*)"|'([^']*)')$/u);
    if (!labelMatch || !valuePart.trim()) {
      supported = false;
      cursor = end;
      previousEnd = end;
      continue;
    }

    const between = body.slice(previousEnd, tierStart).trim();
    let conditions = [];
    if (between.endsWith('?')) {
      const conditionText = between
        .slice(0, -1)
        .trim()
        .replace(/^.*?:/u, '')
        .trim();
      conditions = parseBillingExpressionConditions(conditionText);
      if (conditions === null) {
        supported = false;
        conditions = [];
      }
    }

    const tierValue = valuePart.trim();
    if (BILLING_EXPRESSION_UNSUPPORTED_FUNCTION_REGEX.test(tierValue)) {
      supported = false;
    }
    const prices = parseBillingExpressionTierPrices(tierValue);
    tiers.push({
      label: labelMatch[1] ?? labelMatch[2] ?? '',
      conditions,
      prices,
      hasPrice: BILLING_GUIDE_PRICE_VARIABLES.some(
        (variable) =>
          isFiniteNumber(prices[variable]) && Number(prices[variable]) > 0,
      ),
    });

    cursor = end;
    previousEnd = end;
  }

  if (tiers.length === 0) return null;
  const hasPrice = tiers.some((tier) => tier.hasPrice);
  return { tiers, supported: supported && hasPrice };
};

export const getBillingExpressionInfo = (expression) =>
  parseBillingExpression(expression);

const compareBillingCondition = (left, operator, right) => {
  switch (operator) {
    case '<':
      return left < right;
    case '<=':
      return left <= right;
    case '>':
      return left > right;
    case '>=':
      return left >= right;
    default:
      return false;
  }
};

export const getBillingExpressionTier = (
  expressionOrInfo,
  tokenCounts = {},
) => {
  const info =
    typeof expressionOrInfo === 'string'
      ? parseBillingExpression(expressionOrInfo)
      : expressionOrInfo;
  if (!info?.supported || !Array.isArray(info.tiers)) return null;

  const input = Math.max(0, toFiniteNumber(tokenCounts.input));
  const output = Math.max(0, toFiniteNumber(tokenCounts.output));
  const cacheRead = Math.max(0, toFiniteNumber(tokenCounts.cacheRead));
  const cacheWrite = Math.max(0, toFiniteNumber(tokenCounts.cacheWrite));
  const values = {
    p: input,
    c: output,
    len: input + cacheRead + cacheWrite,
  };
  let fallback = null;
  for (const tier of info.tiers) {
    if (!tier.conditions || tier.conditions.length === 0) {
      fallback = tier;
      continue;
    }
    if (
      tier.conditions.every((condition) =>
        compareBillingCondition(
          values[condition.var],
          condition.op,
          condition.value,
        ),
      )
    ) {
      return tier;
    }
  }
  return fallback || info.tiers[info.tiers.length - 1] || null;
};

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
  models.filter((model) => {
    if (model?.quota_type !== 0) return false;
    if (model?.billing_mode === 'tiered_expr') {
      return Boolean(getBillingExpressionInfo(model?.billing_expr)?.supported);
    }
    return (
      model?.model_ratio !== undefined &&
      model?.model_ratio !== null &&
      model?.model_ratio !== '' &&
      isFiniteNumber(model?.model_ratio)
    );
  });

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

const formatCompactNumber = (value, digits = 4) =>
  Number(value || 0)
    .toFixed(digits)
    .replace(/(\.\d*?[1-9])0+$/u, '$1')
    .replace(/\.0+$/u, '');

export const getBillingDiscountText = (factor, t) => {
  const normalizedFactor = toFiniteNumber(factor, 1);
  if (normalizedFactor < 0.9995) {
    return t('{{discount}}折', {
      discount: formatCompactNumber(normalizedFactor * 10, 1),
    });
  }
  if (normalizedFactor <= 1.0005) return t('原价');
  return t('{{ratio}}倍', {
    ratio: formatCompactNumber(normalizedFactor, 2),
  });
};

export const getBillingDiscountColor = (factor) =>
  toFiniteNumber(factor, 1) < 0.5 ? 'red' : 'green';

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

export const getBillingDynamicUnitPrices = ({
  priceData,
  tokenCounts = {},
  displayPrice,
  currency = 'USD',
  usdExchangeRate = 1,
  customExchangeRate = 1,
  customCurrencySymbol = '¤',
}) => {
  if (!priceData?.isDynamicPricing || !priceData.billingExpr) return null;

  const currencyMeta = getBillingCurrency({
    currency,
    usdExchangeRate,
    customExchangeRate,
    customCurrencySymbol,
  });
  const info = getBillingExpressionInfo(priceData.billingExpr);
  const tier = getBillingExpressionTier(info, tokenCounts);
  if (!tier) return null;

  const groupRatio = getRatioValue(priceData.usedGroupRatio, 1);
  const makePrice = (variable) => {
    const coefficient = Number(tier.prices?.[variable] || 0);
    if (!Number.isFinite(coefficient) || coefficient <= 0) return null;

    const adjustedPrice = coefficient * groupRatio;
    const displayValue =
      typeof displayPrice === 'function'
        ? parseBillingPrice(displayPrice(adjustedPrice))
        : adjustedPrice * currencyMeta.multiplier;
    if (!Number.isFinite(displayValue)) return null;

    return {
      unitPrice: displayValue,
      officialPrice: coefficient * currencyMeta.multiplier,
    };
  };

  return {
    ...currencyMeta,
    input: makePrice('p'),
    output: makePrice('c'),
    cacheRead: makePrice('cr'),
    cacheWrite: makePrice('cc'),
    dynamicTierLabel: tier.label,
    dynamicTierCount: info.tiers.length,
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
