/*
Copyright (C) 2023-2026 QuantumNous

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
import { SettingsPage } from '../components/settings-page'
import type { BillingSettings } from '../types'
import {
  BILLING_DEFAULT_SECTION,
  getBillingSectionContent,
  getBillingSectionMeta,
} from './section-registry.tsx'

const defaultBillingSettings: BillingSettings = {
  QuotaForNewUser: 0,
  PreConsumedQuota: 0,
  QuotaForInviter: 0,
  QuotaForInvitee: 0,
  TopUpLink: '',
  'general_setting.docs_link': '',
  'quota_setting.enable_free_model_pre_consume': true,
  QuotaPerUnit: 500000,
  USDExchangeRate: 7,
  'general_setting.quota_display_type': 'USD',
  'general_setting.custom_currency_symbol': '¤',
  'general_setting.custom_currency_exchange_rate': 1,
  DisplayInCurrencyEnabled: true,
  DisplayTokenStatEnabled: true,
  ModelPrice: '',
  ModelRatio: '',
  CacheRatio: '',
  CreateCacheRatio: '',
  CompletionRatio: '',
  ImageRatio: '',
  AudioRatio: '',
  AudioCompletionRatio: '',
  ExposeRatioEnabled: false,
  'billing_setting.billing_mode': '{}',
  'billing_setting.billing_expr': '{}',
  'tool_price_setting.prices': '{}',
  TopupGroupRatio: '',
  GroupRatio: '',
  UserUsableGroups: '',
  GroupGroupRatio: '',
  AutoGroups: '',
  DefaultUseAutoGroup: false,
  'group_ratio_setting.group_special_usable_group': '{}',
  PayAddress: '',
  EpayId: '',
  EpayKey: '',
  Price: 7.3,
  MinTopUp: 1,
  CustomCallbackAddress: '',
  PayMethods: '',
  'payment_setting.amount_options': '',
  'payment_setting.amount_discount': '',
  'payment_setting.balance_subscription_enabled': true,
  'payment_setting.balance_subscription_promo_enabled': true,
  InvoiceEnabled: false,
  InvoiceTypes: '["personal","company"]',
  InvoiceFeeRules:
    '[{"min":0,"max":500,"type":"fixed","value":50},{"min":501,"max":2000,"type":"fixed","value":100},{"min":2001,"max":5000,"type":"fixed","value":175},{"min":5000,"type":"percent","value":5}]',
  'payment_setting.compliance_confirmed': false,
  'payment_setting.compliance_terms_version': '',
  'payment_setting.compliance_confirmed_at': 0,
  'payment_setting.compliance_confirmed_by': 0,
  'payment_setting.compliance_confirmed_ip': '',
  StripeApiSecret: '',
  StripeWebhookSecret: '',
  StripePriceId: '',
  StripeUnitPrice: 8.0,
  StripeMinTopUp: 1,
  StripePromotionCodesEnabled: false,
  CreemApiKey: '',
  CreemWebhookSecret: '',
  CreemTestMode: false,
  CreemProducts: '[]',
  BepusdtApiUrl: '',
  BepusdtAuthToken: '',
  BepusdtUnitPrice: 7.2,
  BepusdtMinTopUp: 1,
  BepusdtTimeout: 1200,
  BepusdtChains: '[]',
  OkpayGatewayUrl: 'https://api.okaypay.me/shop',
  OkpayMerchantId: '',
  OkpayMerchantToken: '',
  OkpayExchangeRate: 7.2,
  OkpayAutoExchangeEnabled: true,
  OkpayUsdtCnyRate: 7.2,
  OkpayRateApiUrl:
    'https://api.coingecko.com/api/v3/simple/price?ids=tether&vs_currencies=cny&include_last_updated_at=true',
  OkpayRateSource: 'coingecko',
  OkpayOkxSide: 'buy',
  OkpayOkxTier: 3,
  OkpayRateAdjustmentType: 'absolute',
  OkpayRateAdjustmentValue: 0,
  OkpayMinTopUp: 1,
  OkpayCoin: 'USDT',
  WaffoEnabled: false,
  WaffoApiKey: '',
  WaffoPrivateKey: '',
  WaffoPublicCert: '',
  WaffoSandboxPublicCert: '',
  WaffoSandboxApiKey: '',
  WaffoSandboxPrivateKey: '',
  WaffoSandbox: false,
  WaffoMerchantId: '',
  WaffoCurrency: 'USD',
  WaffoUnitPrice: 1,
  WaffoMinTopUp: 1,
  WaffoNotifyUrl: '',
  WaffoReturnUrl: '',
  WaffoPayMethods: '[]',
  WaffoPancakeMerchantID: '',
  WaffoPancakePrivateKey: '',
  WaffoPancakeReturnURL: '',
  WaffoPancakeUnitPrice: 1,
  WaffoPancakeMinTopUp: 1,
  WaffoPancakeStoreID: '',
  WaffoPancakeProductID: '',
  'affiliate_setting.first_level_enabled': false,
  'affiliate_setting.first_level_ratio': 0,
  'affiliate_setting.second_level_enabled': false,
  'affiliate_setting.second_level_ratio': 0,
  'affiliate_setting.settlement_delay_seconds': 0,
  'affiliate_setting.min_withdrawal_amount': 10,
  'affiliate_setting.trigger_topup_enabled': true,
  'affiliate_setting.trigger_subscription_enabled': false,
  'affiliate_setting.filter_redemption_topup_enabled': false,
  'affiliate_setting.payout_methods': 'usdt,alipay,wechat',
  'affiliate_setting.usdt_chain': 'TRC20',
  'affiliate_setting.promotion_template': '邀请链接：{invite_link}',
  'affiliate_setting.review_enabled': false,
  'affiliate_setting.auto_approve_after_days': 0,
  'affiliate_setting.agreement_enabled': false,
  'affiliate_setting.agreement_text': '',
  'affiliate_setting.inviter_min_account_age_days': 0,
  'affiliate_setting.inviter_min_recharge_amount': 0,
  'affiliate_setting.invitee_min_account_age_days': 0,
  'affiliate_setting.invitee_min_recharge_amount': 0,
  'checkin_setting.enabled': false,
  'checkin_setting.min_quota': 1000,
  'checkin_setting.max_quota': 10000,
}

export function BillingSettings() {
  return (
    <SettingsPage
      routePath='/_authenticated/system-settings/billing/$section'
      defaultSettings={defaultBillingSettings}
      defaultSection={BILLING_DEFAULT_SECTION}
      getSectionContent={getBillingSectionContent}
      getSectionMeta={getBillingSectionMeta}
    />
  )
}
