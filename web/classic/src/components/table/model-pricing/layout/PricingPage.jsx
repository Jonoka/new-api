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
import { Layout, ImagePreview } from '@douyinfe/semi-ui';
import PricingSidebar from './PricingSidebar';
import PricingContent from './content/PricingContent';
import ModelDetailSideSheet from '../modal/ModelDetailSideSheet';
import BillingGuide from '../billing/BillingGuide';
import {
  getBillingGuideStorage,
  getBillingGuideModels,
  hasSeenBillingGuide,
  markBillingGuideSeen,
} from '../billing/utils';
import { useModelPricingData } from '../../../../hooks/model-pricing/useModelPricingData';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';

const PricingPage = () => {
  const pricingData = useModelPricingData();
  const { Sider, Content } = Layout;
  const isMobile = useIsMobile();
  const [showRatio, setShowRatio] = React.useState(false);
  const [viewMode, setViewMode] = React.useState('card');
  const [billingWelcomeVisible, setBillingWelcomeVisible] =
    React.useState(false);
  const [billingGuideVisible, setBillingGuideVisible] = React.useState(false);
  const billingGuideCheckedRef = React.useRef(false);
  const billingGuideAvailable = React.useMemo(
    () =>
      pricingData.siteDisplayType !== 'TOKENS' &&
      getBillingGuideModels(pricingData.models).length > 0,
    [pricingData.models, pricingData.siteDisplayType],
  );

  const rememberBillingGuide = React.useCallback(() => {
    const storage = getBillingGuideStorage(
      typeof window !== 'undefined' ? window : undefined,
    );
    markBillingGuideSeen(storage);
  }, []);

  const closeBillingWelcome = React.useCallback(() => {
    setBillingWelcomeVisible(false);
  }, []);

  const openBillingGuide = React.useCallback(() => {
    rememberBillingGuide();
    closeBillingWelcome();
    setBillingGuideVisible(true);
  }, [closeBillingWelcome, rememberBillingGuide]);

  const dismissBillingWelcome = React.useCallback(() => {
    rememberBillingGuide();
    closeBillingWelcome();
  }, [closeBillingWelcome, rememberBillingGuide]);

  React.useEffect(() => {
    if (!billingGuideAvailable) {
      setBillingWelcomeVisible(false);
      setBillingGuideVisible(false);
    }
  }, [billingGuideAvailable]);

  React.useEffect(() => {
    if (
      pricingData.loading ||
      !billingGuideAvailable ||
      billingGuideCheckedRef.current
    ) {
      return;
    }

    billingGuideCheckedRef.current = true;
    const storage = getBillingGuideStorage(
      typeof window !== 'undefined' ? window : undefined,
    );
    if (!hasSeenBillingGuide(storage)) {
      setBillingWelcomeVisible(true);
    }
  }, [pricingData.loading, billingGuideAvailable]);

  const allProps = {
    ...pricingData,
    showRatio,
    setShowRatio,
    viewMode,
    setViewMode,
    onOpenBillingGuide: billingGuideAvailable ? openBillingGuide : undefined,
    billingWelcomeVisible,
    onCloseBillingWelcome: closeBillingWelcome,
    onDismissBillingWelcome: dismissBillingWelcome,
  };

  return (
    <div className='bg-white'>
      <Layout className='pricing-layout'>
        {!isMobile && (
          <Sider className='pricing-scroll-hide pricing-sidebar'>
            <PricingSidebar {...allProps} />
          </Sider>
        )}

        <Content className='pricing-scroll-hide pricing-content'>
          <PricingContent
            {...allProps}
            isMobile={isMobile}
            sidebarProps={allProps}
          />
        </Content>
      </Layout>

      <ImagePreview
        src={pricingData.modalImageUrl}
        visible={pricingData.isModalOpenurl}
        onVisibleChange={(visible) => pricingData.setIsModalOpenurl(visible)}
      />

      <ModelDetailSideSheet
        visible={pricingData.showModelDetail}
        onClose={pricingData.closeModelDetail}
        modelData={pricingData.selectedModel}
        groupRatio={pricingData.groupRatio}
        groupNames={pricingData.groupNames}
        usableGroup={pricingData.usableGroup}
        currency={pricingData.currency}
        siteDisplayType={pricingData.siteDisplayType}
        tokenUnit={pricingData.tokenUnit}
        priceRate={pricingData.priceRate}
        usdExchangeRate={pricingData.usdExchangeRate}
        displayPrice={pricingData.displayPrice}
        showRatio={allProps.showRatio}
        vendorsMap={pricingData.vendorsMap}
        endpointMap={pricingData.endpointMap}
        autoGroups={pricingData.autoGroups}
        t={pricingData.t}
      />

      <BillingGuide
        visible={billingGuideVisible}
        onClose={() => setBillingGuideVisible(false)}
        isMobile={isMobile}
        models={pricingData.models}
        groupRatio={pricingData.groupRatio}
        groupNames={pricingData.groupNames}
        selectedGroup={pricingData.selectedGroup}
        currency={pricingData.currency}
        siteDisplayType={pricingData.siteDisplayType}
        priceRate={pricingData.priceRate}
        usdExchangeRate={pricingData.usdExchangeRate}
        customExchangeRate={pricingData.customExchangeRate}
        customCurrencySymbol={pricingData.customCurrencySymbol}
        displayPrice={pricingData.displayPrice}
        t={pricingData.t}
      />
    </div>
  );
};

export default PricingPage;
