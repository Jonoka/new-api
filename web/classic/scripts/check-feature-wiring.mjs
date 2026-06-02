import fs from 'node:fs'
import path from 'node:path'

const classicRoot = path.resolve(process.cwd(), 'src')
const repoRoot = path.resolve(process.cwd(), '..', '..')

function read(relativePath) {
  return fs.readFileSync(path.join(classicRoot, relativePath), 'utf8')
}

function readRepo(relativePath) {
  return fs.readFileSync(path.join(repoRoot, relativePath), 'utf8')
}

function assertContains(file, needle, message) {
  const content = read(file)
  if (!content.includes(needle)) {
    throw new Error(`${message}\n  file: ${file}\n  missing: ${needle}`)
  }
}

function assertRepoContains(file, needle, message) {
  const content = readRepo(file)
  if (!content.includes(needle)) {
    throw new Error(`${message}\n  file: ${file}\n  missing: ${needle}`)
  }
}

const checks = [
  () => assertContains('App.jsx', '/console/game-center', 'classic 缺少游戏中心路由'),
  () => assertContains('App.jsx', '/console/game-management', 'classic 缺少游戏管理路由'),
  () => assertContains('components/layout/SiderBar.jsx', 'game_center', 'classic 侧边栏缺少游戏中心入口'),
  () => assertContains('components/layout/SiderBar.jsx', 'game_management', 'classic 侧边栏缺少游戏管理入口'),
  () => assertContains('hooks/common/useSidebar.js', 'game_center', 'classic 默认侧边栏配置缺少游戏中心模块'),
  () => assertContains('hooks/common/useSidebar.js', 'game_management', 'classic 默认侧边栏配置缺少游戏管理模块'),
  () => assertRepoContains('controller/misc.go', '"game_center"', '后端 /api/status 侧边栏默认值缺少游戏中心模块'),
  () => assertRepoContains('controller/misc.go', '"game_management"', '后端 /api/status 侧边栏默认值缺少游戏管理模块'),
  () => assertContains('components/settings/RateLimitSetting.jsx', 'ModelRequestRateLimitUserGroup', 'classic 限流设置缺少用户组特殊请求次数字段'),
  () => assertContains('pages/Setting/RateLimit/SettingsRequestRateLimit.jsx', 'ModelRequestRateLimitUserGroup', 'classic 限流表单缺少用户组特殊请求次数字段'),
  () => assertContains('hooks/channels/useChannelsData.jsx', 'CONCURRENCY_LIMIT', 'classic 渠道表缺少并发上限列 key'),
  () => assertContains('components/table/channels/ChannelsColumnDefs.jsx', 'concurrency_limit', 'classic 渠道列定义缺少并发上限字段'),
  () => assertContains('components/table/channels/modals/EditChannelModal.jsx', 'concurrency_limit', 'classic 渠道编辑缺少并发上限字段'),
  () => assertContains('pages/Redemption/index.jsx', '营销福利', 'classic 兑换码页未升级为营销福利页'),
  () => assertContains('pages/Redemption/index.jsx', '优惠码', 'classic 营销福利页缺少优惠码 Tab'),
  () => assertContains('hooks/promo-codes/usePromoCodesData.jsx', '/api/promo-code/', 'classic 缺少优惠码 API 数据 hook'),
  () => assertContains('hooks/promo-codes/usePromoCodesData.jsx', 'searchPromoCodes(searchKeyword, page, pageSize)', 'classic 优惠码搜索分页未传递目标页码'),
  () => assertContains('hooks/promo-codes/usePromoCodesData.jsx', 'searchPromoCodes(searchKeyword, 1, size)', 'classic 优惠码搜索切换分页大小未使用新 pageSize'),
  () => assertContains('components/table/promo-codes/PromoCodesPanel.jsx', '创建优惠码', 'classic 缺少创建优惠码表单入口'),
  () => assertContains('components/table/promo-codes/PromoCodesPanel.jsx', 'editingRecord?.id', 'classic 编辑优惠码提交缺少编辑记录 id 回填'),
  () => assertContains('components/topup/modals/PaymentConfirmModal.jsx', '优惠码', 'classic 余额充值确认弹窗缺少优惠码输入'),
  () => assertContains('components/topup/index.jsx', 'promo_code', 'classic 余额充值请求未传 promo_code'),
  () => assertContains('components/topup/modals/SubscriptionPurchaseModal.jsx', '优惠码', 'classic 套餐购买弹窗缺少优惠码输入'),
  () => assertContains('components/topup/modals/SubscriptionPurchaseModal.jsx', 'onBlur={onPromoCodeBlur}', 'classic 套餐购买优惠码输入缺少离开输入框重新计价'),
  () => assertContains('components/topup/SubscriptionPlansCard.jsx', '/api/subscription/amount', 'classic 套餐购买缺少订阅优惠码金额预览接口调用'),
  () => assertContains('components/topup/SubscriptionPlansCard.jsx', 'promoDiscount', 'classic 套餐购买缺少优惠码折扣金额状态'),
  () => assertContains('components/topup/SubscriptionPlansCard.jsx', 'promo_code', 'classic 套餐购买请求未传 promo_code'),
  () => assertContains('pages/Setting/Payment/SettingsAffiliateCommission.jsx', 'filter_redemption_topup_enabled', 'classic 返佣设置缺少兑换码返佣过滤开关'),
  () => assertRepoContains('setting/affiliate_setting.go', 'FilterRedemptionTopupEnabled', '后端返佣设置缺少兑换码返佣过滤字段'),
]

for (const check of checks) {
  check()
}

console.log('classic feature wiring checks passed')
