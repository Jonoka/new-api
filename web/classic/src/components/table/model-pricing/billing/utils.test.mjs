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

import assert from 'node:assert/strict';
import {
  calculateTokenCost,
  getBillingFactors,
  getBillingGuideGroups,
  getBillingGuideModels,
  getBillingGuideStorage,
  getBillingUnitPricesFromPriceData,
  hasSeenBillingGuide,
  markBillingGuideSeen,
  parseBillingPrice,
  pickBillingGuideGroup,
  pickBillingGuideModel,
} from './utils.js';

const models = [
  {
    model_name: 'fixed-image',
    quota_type: 1,
    model_price: 0.1,
  },
  {
    model_name: 'dynamic',
    quota_type: 0,
    billing_mode: 'tiered_expr',
    model_ratio: 1,
  },
  {
    model_name: 'unpriced',
    quota_type: 0,
    model_ratio: null,
  },
  {
    model_name: 'gpt-5.5',
    quota_type: 0,
    model_ratio: 1.25,
    completion_ratio: 6,
    cache_ratio: 0.1,
    create_cache_ratio: 1.25,
    enable_groups: ['auto', 'standard', 'discount'],
  },
];

assert.deepEqual(getBillingGuideModels(models), [models[3]]);
assert.equal(pickBillingGuideModel(models)?.model_name, 'gpt-5.5');

const groups = getBillingGuideGroups(models[3], {
  standard: 1,
  discount: 0.5,
});
assert.deepEqual(groups, [
  { value: 'standard', ratio: 1 },
  { value: 'discount', ratio: 0.5 },
]);
assert.deepEqual(
  pickBillingGuideGroup(models[3], { standard: 1, discount: 0.5 }),
  { value: 'discount', ratio: 0.5 },
);

const factors = getBillingFactors({
  groupRatio: 0.5,
  priceRate: 1,
  usdExchangeRate: 7,
});
assert.equal(factors.forexFactor, 1 / 7);
assert.equal(factors.compositeFactor, 1 / 14);

const prices = getBillingUnitPricesFromPriceData({
  priceData: {
    isPerToken: true,
    inputPrice: '$1.2500',
    originalInputPrice: '$2.5000',
    completionPrice: '$7.5000',
    originalCompletionPrice: '$15.0000',
    cachePrice: '$0.1250',
    originalCachePrice: '$0.2500',
    createCachePrice: '$1.5625',
    originalCreateCachePrice: '$3.1250',
  },
  currency: 'USD',
});
assert.equal(prices.input.unitPrice, 1.25);
assert.equal(prices.output.unitPrice, 7.5);
assert.equal(prices.cacheRead.unitPrice, 0.125);
assert.equal(prices.cacheWrite.unitPrice, 1.5625);
assert.ok(
  Math.abs(calculateTokenCost(1024, prices.input.unitPrice) - 0.00128) < 1e-12,
);

const rechargePrices = getBillingUnitPricesFromPriceData({
  priceData: {
    isPerToken: true,
    inputPrice: '¥1.2500',
    originalInputPrice: '¥2.5000',
  },
  currency: 'CNY',
  usdExchangeRate: 7,
});
assert.equal(rechargePrices.input.unitPrice, 1.25);
assert.equal(rechargePrices.input.officialPrice, 17.5);
assert.equal(parseBillingPrice('EUR1.25'), 1.25);
assert.equal(parseBillingPrice('AED 1,234.50'), 1234.5);
assert.equal(parseBillingPrice('价格待定'), null);

const storageValues = new Map();
const storage = {
  getItem: (key) => storageValues.get(key),
  setItem: (key, value) => storageValues.set(key, value),
};
assert.equal(hasSeenBillingGuide(storage), false);
assert.equal(markBillingGuideSeen(storage), true);
assert.equal(hasSeenBillingGuide(storage), true);

const throwingStorage = {
  getItem: () => {
    throw new Error('blocked');
  },
  setItem: () => {
    throw new Error('blocked');
  },
};
assert.equal(hasSeenBillingGuide(throwingStorage), false);
assert.equal(markBillingGuideSeen(throwingStorage), false);
assert.equal(markBillingGuideSeen(undefined), false);
assert.equal(
  getBillingGuideStorage({
    get localStorage() {
      throw new Error('blocked');
    },
  }),
  undefined,
);

console.log('billing utils tests passed');
