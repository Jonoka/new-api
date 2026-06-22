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

import React, { useState } from 'react';
import { Tabs, TabPane, Tag } from '@douyinfe/semi-ui';
import { CHANNEL_OPTIONS } from '../../../constants';
import { getChannelIcon, getLobeHubIcon } from '../../../helpers';

const ChannelsTabs = ({
  enableTagMode,
  activeTypeKey,
  setActiveTypeKey,
  channelTypeCounts,
  availableTypeKeys,
  activeVendorKey,
  setActiveVendorKey,
  channelVendorCounts,
  vendors,
  loadChannels,
  searchChannels,
  activePage,
  pageSize,
  idSort,
  setActivePage,
  t,
}) => {
  const [tabMode, setTabMode] = useState('type');

  if (enableTagMode) return null;

  const handleTypeTabChange = (key) => {
    setActiveTypeKey(key);
    setActivePage(1);
    loadChannels(1, pageSize, idSort, enableTagMode, key);
  };

  const handleVendorTabChange = (key) => {
    setActiveVendorKey(key);
    setActivePage(1);
    if (key === 'all') {
      loadChannels(1, pageSize, idSort, enableTagMode);
    } else {
      searchChannels(enableTagMode, 'all', undefined, 1, pageSize, idSort, key);
    }
  };

  const availableVendors = (vendors || []).filter(
    (v) => (channelVendorCounts[v.id] || 0) > 0,
  );

  return (
    <div className='flex flex-col gap-1'>
      <div className='flex items-center gap-2 mb-1'>
        <Tag
          color={tabMode === 'type' ? 'blue' : 'grey'}
          shape='circle'
          className='cursor-pointer select-none'
          onClick={() => {
            if (tabMode === 'type') return;
            setTabMode('type');
            setActiveVendorKey('all');
            setActiveTypeKey('all');
            setActivePage(1);
            loadChannels(1, pageSize, idSort, enableTagMode, 'all');
          }}
        >
          {t('协议类型')}
        </Tag>
        <Tag
          color={tabMode === 'vendor' ? 'blue' : 'grey'}
          shape='circle'
          className='cursor-pointer select-none'
          onClick={() => {
            if (tabMode === 'vendor') return;
            setTabMode('vendor');
            setActiveTypeKey('all');
            setActiveVendorKey('all');
            setActivePage(1);
            loadChannels(1, pageSize, idSort, enableTagMode, 'all');
          }}
        >
          {t('供应商')}
        </Tag>
      </div>

      {tabMode === 'type' && (
        <Tabs
          activeKey={activeTypeKey}
          type='card'
          collapsible
          onChange={handleTypeTabChange}
          className='mb-2'
        >
          <TabPane
            itemKey='all'
            tab={
              <span className='flex items-center gap-2'>
                {t('全部')}
                <Tag
                  color={activeTypeKey === 'all' ? 'red' : 'grey'}
                  shape='circle'
                >
                  {channelTypeCounts['all'] || 0}
                </Tag>
              </span>
            }
          />

          {CHANNEL_OPTIONS.filter((opt) =>
            availableTypeKeys.includes(String(opt.value)),
          ).map((option) => {
            const key = String(option.value);
            const count = channelTypeCounts[option.value] || 0;
            return (
              <TabPane
                key={key}
                itemKey={key}
                tab={
                  <span className='flex items-center gap-2'>
                    {getChannelIcon(option.value)}
                    {option.label}
                    <Tag
                      color={activeTypeKey === key ? 'red' : 'grey'}
                      shape='circle'
                    >
                      {count}
                    </Tag>
                  </span>
                }
              />
            );
          })}
        </Tabs>
      )}

      {tabMode === 'vendor' && (
        <Tabs
          activeKey={activeVendorKey}
          type='card'
          collapsible
          onChange={handleVendorTabChange}
          className='mb-2'
        >
          <TabPane
            itemKey='all'
            tab={
              <span className='flex items-center gap-2'>
                {t('全部')}
                <Tag
                  color={activeVendorKey === 'all' ? 'red' : 'grey'}
                  shape='circle'
                >
                  {channelVendorCounts['all'] || 0}
                </Tag>
              </span>
            }
          />

          {availableVendors.map((vendor) => {
            const key = String(vendor.id);
            const count = channelVendorCounts[vendor.id] || 0;
            return (
              <TabPane
                key={key}
                itemKey={key}
                tab={
                  <span className='flex items-center gap-2'>
                    {getLobeHubIcon(vendor.icon || 'Layers', 14)}
                    {vendor.name}
                    <Tag
                      color={activeVendorKey === key ? 'red' : 'grey'}
                      shape='circle'
                    >
                      {count}
                    </Tag>
                  </span>
                }
              />
            );
          })}
        </Tabs>
      )}
    </div>
  );
};

export default ChannelsTabs;
