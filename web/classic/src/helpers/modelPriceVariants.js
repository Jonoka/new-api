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

/**
 * @typedef {Object} ModelPriceVariantRule
 * @property {string=} resolution 分辨率档位
 * @property {string=} quality 质量档位
 * @property {number} price 对应规格的最终单价
 */

/**
 * @typedef {Object} ModelPriceVariantsConfig
 * @property {boolean} resolution_enabled 是否按分辨率匹配
 * @property {boolean} quality_enabled 是否按质量匹配
 * @property {ModelPriceVariantRule[]=} rules 规格最终单价规则
 * @property {boolean=} inherited 是否继承后端内置配置
 */

const RESOLUTION_ALIASES = {
  480: '480p',
  '480p': '480p',
  sd: '480p',
  720: '720p',
  '720p': '720p',
  hd: '720p',
  1080: '1080p',
  '1080p': '1080p',
  fhd: '1080p',
  'full-hd': '1080p',
  full_hd: '1080p',
  '2k': '2k',
  '4k': '4k',
};

const isPlainObject = (value) =>
  value !== null && typeof value === 'object' && !Array.isArray(value);

const hasOwn = (value, key) => Object.prototype.hasOwnProperty.call(value, key);

const isBlank = (value) =>
  value === null ||
  value === undefined ||
  (typeof value === 'string' && value.trim() === '');

const translate = (t, key, params) =>
  typeof t === 'function' ? t(key, params) : key;

const toEditableText = (value) => {
  if (value === null || value === undefined) return '';
  return String(value);
};

const toEditablePrice = (value) => {
  if (isBlank(value)) return '';
  const numberValue = Number(value);
  return Number.isFinite(numberValue) ? String(numberValue) : String(value);
};

export const isGrokImagineVideoModel = (modelName = '') =>
  String(modelName).toLowerCase().startsWith('grok-imagine-video');

export const isBuiltInGrokImagineVideoModel = (modelName = '') =>
  ['grok-imagine-video', 'grok-imagine-video-1.5'].includes(
    String(modelName).trim().toLowerCase(),
  );

export const normalizeVariantResolution = (value) => {
  const normalized = toEditableText(value).trim().toLowerCase();
  return RESOLUTION_ALIASES[normalized] || normalized;
};

export const normalizeVariantQuality = (value) =>
  toEditableText(value).trim().toLowerCase();

/**
 * 将接口配置收敛为稳定的 snake_case 结构。
 * 无效规则不会在这里丢弃，编辑器仍需展示并阻止提交。
 *
 * @param {unknown} rawConfig
 * @returns {ModelPriceVariantsConfig|null}
 */
export const normalizeModelPriceVariantsConfig = (rawConfig) => {
  if (!isPlainObject(rawConfig)) return null;

  const normalized = {
    resolution_enabled: rawConfig.resolution_enabled === true,
    quality_enabled: rawConfig.quality_enabled === true,
    rules: Array.isArray(rawConfig.rules)
      ? rawConfig.rules.map((rawRule) => {
          const rule = isPlainObject(rawRule) ? rawRule : {};
          return {
            ...(hasOwn(rule, 'resolution')
              ? { resolution: toEditableText(rule.resolution) }
              : {}),
            ...(hasOwn(rule, 'quality')
              ? { quality: toEditableText(rule.quality) }
              : {}),
            price: hasOwn(rule, 'price') ? rule.price : null,
          };
        })
      : [],
  };

  if (hasOwn(rawConfig, 'inherited')) {
    normalized.inherited = rawConfig.inherited === true;
  }

  return normalized;
};

export const createEmptyModelPriceVariantsState = () => ({
  configured: false,
  inherited: false,
  restoreInherited: false,
  resolutionEnabled: false,
  qualityEnabled: false,
  rules: [],
});

export const createEmptyModelPriceVariantRule = () => ({
  resolution: '',
  quality: '',
  price: '',
});

export const createModelPriceVariantsState = (rawConfig, modelName = '') => {
  const config = normalizeModelPriceVariantsConfig(rawConfig);
  if (!config) return createEmptyModelPriceVariantsState();

  const hideQuality = isGrokImagineVideoModel(modelName);
  return {
    configured: true,
    inherited: config.inherited === true,
    restoreInherited: false,
    resolutionEnabled: config.resolution_enabled,
    qualityEnabled: hideQuality ? false : config.quality_enabled,
    rules: config.rules.map((rule) => ({
      resolution: toEditableText(rule.resolution),
      quality: hideQuality ? '' : toEditableText(rule.quality),
      price: toEditablePrice(rule.price),
    })),
  };
};

export const cloneModelPriceVariantsState = (
  state,
  modelName = '',
  { markExplicit = false } = {},
) => {
  const source = state || createEmptyModelPriceVariantsState();
  const hideQuality = isGrokImagineVideoModel(modelName);
  return {
    configured: Boolean(source.configured),
    inherited: markExplicit ? false : source.inherited === true,
    restoreInherited: markExplicit ? false : Boolean(source.restoreInherited),
    resolutionEnabled: Boolean(source.resolutionEnabled),
    qualityEnabled: hideQuality ? false : Boolean(source.qualityEnabled),
    rules: Array.isArray(source.rules)
      ? source.rules.map((rule) => ({
          resolution: toEditableText(rule?.resolution),
          quality: hideQuality ? '' : toEditableText(rule?.quality),
          price: toEditablePrice(rule?.price),
        }))
      : [],
  };
};

const serializeModelPriceVariantsInternal = (
  model,
  t,
  { enforceGrokQuality = true } = {},
) => {
  const source = model?.priceVariants || createEmptyModelPriceVariantsState();
  if (source.restoreInherited) return null;
  const modelName = String(model?.name || '').trim();
  const hideQuality = enforceGrokQuality && isGrokImagineVideoModel(modelName);
  const resolutionEnabled = Boolean(source.resolutionEnabled);
  const qualityEnabled = hideQuality ? false : Boolean(source.qualityEnabled);
  const configured =
    Boolean(source.configured) ||
    source.inherited === true ||
    resolutionEnabled ||
    qualityEnabled;

  if (!configured) return null;

  const result = {
    resolution_enabled: resolutionEnabled,
    quality_enabled: qualityEnabled,
    inherited: source.inherited === true,
  };

  if (!resolutionEnabled && !qualityEnabled) {
    return result;
  }

  if (isBlank(model?.fixedPrice)) {
    throw new Error(
      translate(
        t,
        '模型 {{name}} 开启规格差异计费时必须填写固定价格，作为未匹配规格的兜底价格',
        { name: modelName },
      ),
    );
  }

  const rules = Array.isArray(source.rules) ? source.rules : [];
  if (rules.length === 0) {
    throw new Error(
      translate(t, '模型 {{name}} 已开启规格差异计费，但没有配置任何价格规则', {
        name: modelName,
      }),
    );
  }

  const seen = new Set();
  result.rules = rules.map((rule, index) => {
    const ruleNumber = index + 1;
    const resolution = resolutionEnabled
      ? normalizeVariantResolution(rule?.resolution)
      : '';
    const quality = qualityEnabled
      ? normalizeVariantQuality(rule?.quality)
      : '';

    if (resolutionEnabled && !resolution) {
      throw new Error(
        translate(t, '模型 {{name}} 第 {{index}} 条规则的分辨率不能为空', {
          name: modelName,
          index: ruleNumber,
        }),
      );
    }
    if (qualityEnabled && !quality) {
      throw new Error(
        translate(t, '模型 {{name}} 第 {{index}} 条规则的质量档位不能为空', {
          name: modelName,
          index: ruleNumber,
        }),
      );
    }
    if (isBlank(rule?.price)) {
      throw new Error(
        translate(t, '模型 {{name}} 第 {{index}} 条规则的价格不能为空', {
          name: modelName,
          index: ruleNumber,
        }),
      );
    }

    const price = Number(rule.price);
    if (!Number.isFinite(price) || price < 0) {
      throw new Error(
        translate(t, '模型 {{name}} 第 {{index}} 条规则的价格无效', {
          name: modelName,
          index: ruleNumber,
        }),
      );
    }

    const combinationKey = `${resolution}\u0000${quality}`;
    const combination = [resolution, quality].filter(Boolean).join(' / ');
    if (seen.has(combinationKey)) {
      throw new Error(
        translate(t, '模型 {{name}} 存在重复的规格组合：{{combination}}', {
          name: modelName,
          combination,
        }),
      );
    }
    seen.add(combinationKey);

    return {
      ...(resolutionEnabled ? { resolution } : {}),
      ...(qualityEnabled ? { quality } : {}),
      price,
    };
  });

  return result;
};

export const serializeModelPriceVariants = (model, t) =>
  serializeModelPriceVariantsInternal(model, t);

export const buildModelPriceVariantsPreview = (model) => {
  const source = model?.priceVariants || createEmptyModelPriceVariantsState();
  if (source.restoreInherited) return null;
  const hideQuality = isGrokImagineVideoModel(model?.name);
  const resolutionEnabled = Boolean(source.resolutionEnabled);
  const qualityEnabled = hideQuality ? false : Boolean(source.qualityEnabled);
  const configured =
    Boolean(source.configured) ||
    source.inherited === true ||
    resolutionEnabled ||
    qualityEnabled;

  if (!configured) return null;

  const preview = {
    resolution_enabled: resolutionEnabled,
    quality_enabled: qualityEnabled,
    inherited: source.inherited === true,
  };

  if (resolutionEnabled || qualityEnabled) {
    preview.rules = (source.rules || []).map((rule) => {
      const numericPrice = Number(rule?.price);
      return {
        ...(resolutionEnabled
          ? { resolution: toEditableText(rule?.resolution).trim() }
          : {}),
        ...(qualityEnabled
          ? { quality: toEditableText(rule?.quality).trim() }
          : {}),
        price:
          !isBlank(rule?.price) && Number.isFinite(numericPrice)
            ? numericPrice
            : toEditableText(rule?.price),
      };
    });
  }

  return preview;
};

export const getModelPriceVariantsJSONError = (rawValue, t) => {
  let parsed;
  try {
    parsed = JSON.parse(rawValue);
  } catch (_error) {
    return translate(t, '不是合法的 JSON 字符串');
  }

  if (!isPlainObject(parsed)) {
    return translate(t, '规格差异计费配置必须是 JSON 对象');
  }

  try {
    Object.entries(parsed).forEach(([rawModelName, rawConfig]) => {
      const modelName = rawModelName.trim();
      if (!modelName || !isPlainObject(rawConfig)) {
        throw new Error(
          translate(
            t,
            '规格差异计费配置无效，请检查模型名称、档位、价格和重复组合。',
          ),
        );
      }

      for (const key of [
        'resolution_enabled',
        'quality_enabled',
        'inherited',
      ]) {
        if (hasOwn(rawConfig, key) && typeof rawConfig[key] !== 'boolean') {
          throw new Error(
            translate(
              t,
              '规格差异计费配置无效，请检查模型名称、档位、价格和重复组合。',
            ),
          );
        }
      }

      // 后端会忽略继承项；无需把内置配置固化成显式覆盖。
      if (rawConfig.inherited === true) return;

      if (hasOwn(rawConfig, 'rules') && !Array.isArray(rawConfig.rules)) {
        throw new Error(
          translate(
            t,
            '规格差异计费配置无效，请检查模型名称、档位、价格和重复组合。',
          ),
        );
      }

      const config = normalizeModelPriceVariantsConfig(rawConfig);
      serializeModelPriceVariantsInternal(
        {
          name: modelName,
          fixedPrice: '0',
          priceVariants: {
            configured: true,
            inherited: false,
            resolutionEnabled: config.resolution_enabled,
            qualityEnabled: config.quality_enabled,
            rules: config.rules.map((rule) => ({
              resolution: toEditableText(rule.resolution),
              quality: toEditableText(rule.quality),
              price: toEditablePrice(rule.price),
            })),
          },
        },
        t,
        { enforceGrokQuality: false },
      );
    });
  } catch (error) {
    return error.message;
  }

  return '';
};

/**
 * 提取模型广场可展示的有效规格规则和原始价格范围。
 *
 * @param {unknown} basePrice
 * @param {unknown} rawConfig
 * @returns {{config: ModelPriceVariantsConfig, rules: ModelPriceVariantRule[], minPrice: number, maxPrice: number}|null}
 */
export const getModelPriceVariantRange = (basePrice, rawConfig) => {
  const config = normalizeModelPriceVariantsConfig(rawConfig);
  if (
    !config ||
    (!config.resolution_enabled && !config.quality_enabled) ||
    config.rules.length === 0
  ) {
    return null;
  }

  const rules = config.rules
    .map((rule) => {
      const resolution = config.resolution_enabled
        ? normalizeVariantResolution(rule.resolution)
        : '';
      const quality = config.quality_enabled
        ? normalizeVariantQuality(rule.quality)
        : '';
      const price = Number(rule.price);
      if (
        (config.resolution_enabled && !resolution) ||
        (config.quality_enabled && !quality) ||
        isBlank(rule.price) ||
        !Number.isFinite(price) ||
        price < 0
      ) {
        return null;
      }
      return {
        ...(config.resolution_enabled ? { resolution } : {}),
        ...(config.quality_enabled ? { quality } : {}),
        price,
      };
    })
    .filter(Boolean);

  if (rules.length === 0) return null;
  const parsedBasePrice = Number(basePrice);
  const normalizedBasePrice = Number.isFinite(parsedBasePrice)
    ? Math.max(parsedBasePrice, 0)
    : 0;
  const prices = [normalizedBasePrice, ...rules.map((rule) => rule.price)];
  return {
    config,
    rules,
    minPrice: Math.min(...prices),
    maxPrice: Math.max(...prices),
  };
};

export const getModelPriceVariantRuleLabel = (rule, config, t) => {
  const labels = [];
  if (config?.resolution_enabled) {
    labels.push(`${translate(t, '分辨率')} ${rule.resolution}`);
  }
  if (config?.quality_enabled) {
    labels.push(`${translate(t, '质量档位')} ${rule.quality}`);
  }
  return labels.join(' / ');
};
