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
import { Tabs, Typography } from '@douyinfe/semi-ui';
import { Gift } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import PromoCodesPanel from '../../components/table/promo-codes/PromoCodesPanel';
import RedemptionsTable from '../../components/table/redemptions';

const { Title } = Typography;

const Redemption = () => {
  const { t } = useTranslation();

  return (
    <div className='mt-[60px] px-2'>
      <div className='flex items-center gap-2 mb-4 text-orange-500'>
        <Gift size={20} />
        <Title heading={3} className='!mb-0'>
          {t('营销福利')}
        </Title>
      </div>

      <Tabs type='line' defaultActiveKey='redemptions'>
        <Tabs.TabPane tab={t('兑换码')} itemKey='redemptions'>
          <RedemptionsTable />
        </Tabs.TabPane>
        <Tabs.TabPane tab={t('优惠码')} itemKey='promo-codes'>
          <PromoCodesPanel />
        </Tabs.TabPane>
      </Tabs>
    </div>
  );
};

export default Redemption;
