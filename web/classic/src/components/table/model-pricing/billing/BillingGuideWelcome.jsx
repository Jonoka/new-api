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
import { Button } from '@douyinfe/semi-ui';
import { IconHelpCircle } from '@douyinfe/semi-icons';

const BILLING_GUIDE_MASK_STYLE = {
  backgroundColor: 'rgba(15, 23, 42, 0.58)',
  backdropFilter: 'blur(5px)',
  WebkitBackdropFilter: 'blur(5px)',
};

const BillingGuideWelcome = ({ onViewDetails, onDismiss, t }) => (
  <div className='flex w-full flex-col items-center p-1 text-center'>
    <div
      className='flex h-10 w-10 items-center justify-center rounded-full'
      style={{
        color: 'var(--semi-color-primary)',
        backgroundColor: 'var(--semi-color-primary-light-default)',
      }}
    >
      <IconHelpCircle size='large' />
    </div>

    <div
      className='mt-3 text-base font-semibold'
      style={{ color: 'var(--semi-color-text-0)' }}
    >
      {t('模型广场如何计费？')}
    </div>
    <div
      className='mt-1.5 text-sm leading-5'
      style={{ color: 'var(--semi-color-text-2)' }}
    >
      {t('用两分钟了解折扣从哪里来、费用如何计算，以及完整的计费逻辑。')}
    </div>

    <div className='mt-4 flex w-full flex-col gap-2'>
      <Button block theme='solid' type='primary' onClick={onViewDetails}>
        {t('查看计费说明')}
      </Button>
      <Button block theme='light' type='tertiary' onClick={onDismiss}>
        {t('不再显示')}
      </Button>
    </div>
  </div>
);

export { BILLING_GUIDE_MASK_STYLE };
export default BillingGuideWelcome;
