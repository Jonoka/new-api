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
  Button,
  Card,
  Input,
  Switch,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { IconDelete, IconPlus } from '@douyinfe/semi-icons';
import {
  createEmptyModelPriceVariantsState,
  isBuiltInGrokImagineVideoModel,
  isGrokImagineVideoModel,
} from '../../../../helpers';

const { Text } = Typography;

export default function ModelPriceVariantsEditor({
  model,
  isMobile,
  onDimensionChange,
  onRuleChange,
  onAddRule,
  onDeleteRule,
  onRestoreInherited,
  t,
}) {
  const variants = model.priceVariants || createEmptyModelPriceVariantsState();
  const isGrokVideo = isGrokImagineVideoModel(model.name);
  const dimensionsEnabled =
    variants.resolutionEnabled || variants.qualityEnabled;
  const canRestoreInherited =
    isBuiltInGrokImagineVideoModel(model.name) &&
    !variants.inherited &&
    !variants.restoreInherited &&
    (variants.configured || variants.rules.length > 0);
  const priceSuffix = t(model.priceUnit === 'second' ? '$/秒' : '$/次');
  const fieldCount =
    Number(variants.resolutionEnabled) + Number(variants.qualityEnabled) + 1;

  return (
    <Card
      bodyStyle={{ padding: 16 }}
      style={{
        marginTop: 16,
        marginBottom: 16,
        background: 'var(--semi-color-fill-0)',
      }}
    >
      <div className='flex items-start justify-between gap-3 mb-4'>
        <div>
          <div className='font-medium'>{t('规格差异计费')}</div>
          <div className='text-xs text-gray-500 mt-1'>
            {t(
              '规则价格是对应规格的最终单价，不会与固定价格叠加；未匹配任何规则时使用固定价格兜底。',
            )}
          </div>
        </div>
        <div className='flex shrink-0 items-center gap-2'>
          {variants.inherited ? (
            <Tag color='green' shape='circle'>
              {t('继承默认')}
            </Tag>
          ) : null}
          {variants.restoreInherited ? (
            <Tag color='green' shape='circle'>
              {t('恢复内置配置')}
            </Tag>
          ) : null}
          {canRestoreInherited && !variants.inherited ? (
            <Button
              size='small'
              theme='borderless'
              onClick={onRestoreInherited}
            >
              {t('恢复内置配置')}
            </Button>
          ) : null}
        </div>
      </div>

      <div
        style={{
          display: 'grid',
          gridTemplateColumns:
            isMobile || isGrokVideo
              ? 'minmax(0, 1fr)'
              : 'repeat(2, minmax(0, 1fr))',
          gap: 12,
          marginBottom: 12,
        }}
      >
        <div className='flex items-center justify-between rounded-lg border border-gray-200 px-3 py-2'>
          <Text>{t('按分辨率区分')}</Text>
          <Switch
            size='small'
            checked={variants.resolutionEnabled}
            onChange={(checked) =>
              onDimensionChange('resolutionEnabled', checked)
            }
          />
        </div>
        {!isGrokVideo ? (
          <div className='flex items-center justify-between rounded-lg border border-gray-200 px-3 py-2'>
            <Text>{t('按质量档位区分')}</Text>
            <Switch
              size='small'
              checked={variants.qualityEnabled}
              onChange={(checked) =>
                onDimensionChange('qualityEnabled', checked)
              }
            />
          </div>
        ) : null}
      </div>

      {isGrokVideo ? (
        <div className='text-xs text-gray-500 mb-3'>
          {t('Grok 视频模型的清晰度就是分辨率，无需单独配置质量档位。')}
        </div>
      ) : null}

      {dimensionsEnabled ? (
        <>
          <div className='flex items-center justify-between gap-3 mb-3'>
            <Text strong>{t('档位最终单价')}</Text>
            <Button size='small' icon={<IconPlus />} onClick={onAddRule}>
              {t('新增价格规则')}
            </Button>
          </div>

          {variants.rules.length === 0 ? (
            <div className='rounded-lg bg-gray-50 px-3 py-4 text-center text-sm text-gray-500'>
              {t('暂无价格规则，请新增至少一条规则。')}
            </div>
          ) : (
            <div className='flex flex-col gap-3'>
              {variants.rules.map((rule, index) => (
                <div
                  key={index}
                  className='rounded-lg border border-gray-200 p-3'
                >
                  <div
                    style={{
                      display: 'grid',
                      gridTemplateColumns: isMobile
                        ? 'minmax(0, 1fr)'
                        : `repeat(${fieldCount}, minmax(0, 1fr)) auto`,
                      gap: 10,
                      alignItems: 'end',
                    }}
                  >
                    {variants.resolutionEnabled ? (
                      <div>
                        <div className='text-xs text-gray-600 mb-1'>
                          {t('分辨率')}
                        </div>
                        <Input
                          value={rule.resolution}
                          placeholder={t('例如 480p')}
                          onChange={(value) =>
                            onRuleChange(index, 'resolution', value)
                          }
                        />
                      </div>
                    ) : null}
                    {variants.qualityEnabled ? (
                      <div>
                        <div className='text-xs text-gray-600 mb-1'>
                          {t('质量档位')}
                        </div>
                        <Input
                          value={rule.quality}
                          placeholder={t('例如 standard')}
                          onChange={(value) =>
                            onRuleChange(index, 'quality', value)
                          }
                        />
                      </div>
                    ) : null}
                    <div>
                      <div className='text-xs text-gray-600 mb-1'>
                        {t('最终单价')}
                      </div>
                      <Input
                        value={rule.price}
                        placeholder={t('输入档位价格')}
                        suffix={priceSuffix}
                        onChange={(value) =>
                          onRuleChange(index, 'price', value)
                        }
                      />
                    </div>
                    <Button
                      type='danger'
                      theme='borderless'
                      icon={<IconDelete />}
                      aria-label={t('删除价格规则')}
                      onClick={() => onDeleteRule(index)}
                    />
                  </div>
                </div>
              ))}
            </div>
          )}
        </>
      ) : (
        <div className='rounded-lg bg-gray-50 px-3 py-3 text-sm text-gray-500'>
          {t('开启至少一个规格维度后，可以配置各档位的最终单价。')}
        </div>
      )}
    </Card>
  );
}
