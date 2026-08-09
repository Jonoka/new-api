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

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  Banner,
  Button,
  Card,
  Collapsible,
  DatePicker,
  Input,
  Select,
  Space,
  Tabs,
  Tag,
  Toast,
  Typography,
} from "@douyinfe/semi-ui";
import {
  Activity,
  ChevronDown,
  RefreshCw,
  RotateCcw,
  SlidersHorizontal,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { API } from "../../helpers";
import ChannelsView from "./ChannelsView";
import FailuresView from "./FailuresView";
import OperationsView from "./OperationsView";
import OverviewView from "./OverviewView";
import ProbeView from "./ProbeView";
import {
  API_ROOT,
  OUTCOME_LABELS,
  appendCommonParams,
  buildQualityMessages,
  formatRelativeTime,
  getFormattingLocale,
  getErrorMessage,
  modelOption,
  resolveRange,
  setFormattingLocale,
} from "./utils";

const { Text, Title } = Typography;

const DEFAULT_FILTERS = {
  channelId: "",
  channelType: "",
  group: "",
  requestedModelValue: "",
  requestedModel: "",
  requestedModelHash: "",
  upstreamModelValue: "",
  upstreamModel: "",
  upstreamModelHash: "",
  outcome: "",
  statusCode: "",
  stream: "",
  trafficSource: "relay",
  dataOrigin: "live,legacy",
};

const VIEW_HINTS = {
  overview: "状态码筛选只在“状态码与失败”视图生效。",
  operations:
    "矩阵固定比较多个重叠时间窗；复合维度按层级逐级展开并懒加载子项。状态码筛选不参与矩阵。",
  channels: "状态码筛选不适用于渠道尝试聚合；展开模型时按所选模型口径统计。",
  failures:
    "响应方式只影响状态码分布，失败明细暂不支持该筛选；历史明细来自脱敏日志推导。",
  probe:
    "主动探测只应用渠道、渠道类型和请求模型；时间及其他业务筛选不参与探测。",
};

const ChannelObservability = () => {
  const { t, i18n } = useTranslation();
  setFormattingLocale(i18n.resolvedLanguage || i18n.language);
  const [activeView, setActiveView] = useState("overview");
  const [range, setRange] = useState("today");
  const [customRange, setCustomRange] = useState([]);
  const [granularity, setGranularity] = useState("auto");
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [filters, setFilters] = useState(DEFAULT_FILTERS);
  const [filterOptions, setFilterOptions] = useState({
    channels: [],
    channelTypes: [],
    groups: [],
    requestedModels: [],
    upstreamModels: [],
    outcomes: [],
  });
  const [retentionDays, setRetentionDays] = useState(7);
  const [refreshKey, setRefreshKey] = useState(0);
  const [filtersRefreshKey, setFiltersRefreshKey] = useState(0);
  const [filterLoading, setFilterLoading] = useState(false);
  const [viewStatus, setViewStatus] = useState({
    loading: true,
    error: "",
    meta: null,
    empty: false,
  });
  const [operationsPreset, setOperationsPreset] = useState(null);
  const modelSearchControllers = useRef(new Map());
  const modelSearchTimers = useRef(new Map());

  useEffect(() => {
    const controller = new AbortController();
    const loadFilters = async () => {
      setFilterLoading(true);
      try {
        const response = await API.get(`${API_ROOT}/filters`, {
          signal: controller.signal,
          skipErrorHandler: true,
        });
        const payload = response.data?.data || {};
        setFilterOptions((current) => ({
          ...current,
          channels: payload.channels || [],
          channelTypes: payload.channel_types || [],
          groups: payload.groups || [],
          requestedModels: (
            payload.requested_model_options ||
            payload.requested_models ||
            []
          ).map(modelOption),
          upstreamModels: (
            payload.upstream_model_options ||
            payload.upstream_models ||
            []
          ).map(modelOption),
          outcomes: payload.outcomes || Object.keys(OUTCOME_LABELS),
        }));
        if (Number(payload.meta?.retention_days) > 0) {
          setRetentionDays(Number(payload.meta.retention_days));
        }
      } catch (error) {
        if (error?.name !== "CanceledError" && !controller.signal.aborted) {
          Toast.warning({
            content: t("部分筛选项加载失败：{{message}}", {
              message: getErrorMessage(error, t("未知错误")),
            }),
          });
        }
      } finally {
        if (!controller.signal.aborted) setFilterLoading(false);
      }
    };
    loadFilters();
    return () => controller.abort();
  }, [filtersRefreshKey, t]);

  useEffect(
    () => () => {
      modelSearchControllers.current.forEach((controller) =>
        controller.abort(),
      );
      modelSearchTimers.current.forEach((timer) => window.clearTimeout(timer));
    },
    [],
  );

  const updateFilter = useCallback((key, value) => {
    setFilters((current) => ({ ...current, [key]: value }));
  }, []);

  const changeModel = (dimension, value) => {
    const key = dimension === "upstream" ? "upstreamModels" : "requestedModels";
    const option = filterOptions[key].find((item) => item.value === value);
    setFilters((current) => ({
      ...current,
      [`${dimension}ModelValue`]: value || "",
      [`${dimension}Model`]: option?.model || "",
      [`${dimension}ModelHash`]: option?.hash || "",
    }));
  };

  const searchModels = (dimension, query) => {
    const previousTimer = modelSearchTimers.current.get(dimension);
    if (previousTimer) window.clearTimeout(previousTimer);
    const timer = window.setTimeout(async () => {
      modelSearchControllers.current.get(dimension)?.abort();
      const controller = new AbortController();
      modelSearchControllers.current.set(dimension, controller);
      try {
        const response = await API.get(`${API_ROOT}/filters/models`, {
          params: {
            model_dimension: dimension,
            q: String(query || "").trim(),
            page: 1,
            page_size: 100,
          },
          signal: controller.signal,
          skipErrorHandler: true,
        });
        const options = (response.data?.data?.items || []).map(modelOption);
        const key =
          dimension === "upstream" ? "upstreamModels" : "requestedModels";
        setFilterOptions((current) => {
          const selectedValue = filters[`${dimension}ModelValue`];
          const selected = current[key].find(
            (item) => item.value === selectedValue,
          );
          const merged =
            selected && !options.some((item) => item.value === selected.value)
              ? [selected, ...options]
              : options;
          return { ...current, [key]: merged };
        });
      } catch (error) {
        if (error?.name !== "CanceledError" && !controller.signal.aborted) {
          Toast.error({ content: getErrorMessage(error, t("模型搜索失败")) });
        }
      }
    }, 260);
    modelSearchTimers.current.set(dimension, timer);
  };

  const queryKey = useMemo(
    () =>
      JSON.stringify({
        range,
        customRange: customRange.map((value) => value?.getTime?.() || 0),
        granularity,
        filters,
      }),
    [customRange, filters, granularity, range],
  );

  const makeParams = useCallback(
    (extra = {}, options = {}) => {
      const params = appendCommonParams(
        new URLSearchParams(),
        { filters, range, customRange, granularity },
        options,
      );
      Object.entries(extra).forEach(([key, value]) => {
        if (value !== undefined && value !== null && value !== "") {
          params.set(key, String(value));
        }
      });
      return params;
    },
    [customRange, filters, granularity, range],
  );

  const handleViewStatus = useCallback((status) => {
    setViewStatus((current) => ({ ...current, ...status }));
  }, []);

  const resetFilters = () => {
    setRange("today");
    setCustomRange([]);
    setGranularity("auto");
    setFilters(DEFAULT_FILTERS);
  };

  const refresh = () => {
    setRefreshKey((value) => value + 1);
    setFiltersRefreshKey((value) => value + 1);
  };

  const chooseRange = (nextRange) => {
    if (nextRange === "custom" && customRange.length !== 2) {
      const end = new Date();
      setCustomRange([new Date(end.getTime() - 3600 * 1000), end]);
    }
    setRange(nextRange);
  };

  const changeCustomRange = (value) => {
    if (!Array.isArray(value) || value.length !== 2) {
      setCustomRange([]);
      return;
    }
    const start = value[0]?.getTime?.();
    const end = value[1]?.getTime?.();
    if (!start || !end || end <= start) return;
    if (end > Date.now()) {
      Toast.warning({ content: t("结束时间不能晚于当前时间") });
      return;
    }
    if (end - start > retentionDays * 86400 * 1000) {
      Toast.warning({
        content: t("当前统计最多支持 {{days}} 天范围", { days: retentionDays }),
      });
      return;
    }
    setCustomRange(value);
  };

  const handleGroupChannels = useCallback((modelDimension) => {
    setOperationsPreset({
      version: Date.now(),
      dimension: "group_channel_model",
      modelDimension,
    });
    setActiveView("operations");
  }, []);

  const handleShowFailures = useCallback((channelId) => {
    setFilters((current) => ({ ...current, channelId: String(channelId) }));
    setActiveView("failures");
  }, []);

  const [queryStart] = resolveRange(range, customRange);
  const qualityMessages = buildQualityMessages(
    viewStatus.meta,
    queryStart,
    filters.dataOrigin,
    t,
  );
  const freshness = viewStatus.loading
    ? t("正在刷新统计数据")
    : viewStatus.error
      ? t("最近刷新失败")
      : viewStatus.meta?.last_flushed_at || viewStatus.meta?.generated_at
        ? t("数据更新于 {{time}}", {
            time: formatRelativeTime(
              viewStatus.meta.last_flushed_at || viewStatus.meta.generated_at,
            ),
          })
        : t("已完成刷新");

  const commonViewProps = {
    makeParams,
    queryKey,
    refreshKey,
    onStatus: handleViewStatus,
  };

  return (
    <div className="mx-auto mt-[60px] min-h-screen w-full max-w-[1600px] px-2 pb-8 lg:min-h-0">
      <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-3">
          <div
            className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md"
            style={{
              color: "var(--semi-color-primary)",
              backgroundColor: "var(--semi-color-primary-light-default)",
            }}
          >
            <Activity size={20} />
          </div>
          <div>
            <Title heading={4}>{t("渠道可观测性中心")}</Title>
            <Text type="tertiary">
              {t("按分组、模型和渠道观察多时间窗稳定性")}
            </Text>
          </div>
        </div>
        <Space>
          <Tag
            color={
              viewStatus.error ? "red" : viewStatus.loading ? "blue" : "green"
            }
          >
            {freshness}
          </Tag>
          <Button
            type="primary"
            icon={<RefreshCw size={16} />}
            loading={viewStatus.loading}
            onClick={refresh}
          >
            {t("刷新")}
          </Button>
        </Space>
      </div>

      <div className="overflow-x-auto scrollbar-hide">
        <Tabs
          type="line"
          activeKey={activeView}
          onChange={setActiveView}
          collapsible
          tabBarClassName="whitespace-nowrap"
          tabBarStyle={{ flexWrap: "nowrap", overflowX: "auto" }}
        >
          <Tabs.TabPane tab={t("总览")} itemKey="overview" />
          <Tabs.TabPane tab={t("运维矩阵")} itemKey="operations" />
          <Tabs.TabPane tab={t("渠道与模型")} itemKey="channels" />
          <Tabs.TabPane tab={t("状态码与失败")} itemKey="failures" />
          <Tabs.TabPane tab={t("主动探测")} itemKey="probe" />
        </Tabs>
      </div>

      <Card bodyStyle={{ padding: 12 }} className="mb-4">
        <div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
          <Space wrap>
            {[
              ["1h", "最近 1 小时"],
              ["today", "今天"],
              ["yesterday", "昨天"],
              ["7d", "最近 7 天"],
              ["custom", "自定义"],
            ].map(([value, label]) => (
              <Button
                key={value}
                size="small"
                type={range === value ? "primary" : "tertiary"}
                theme={range === value ? "solid" : "light"}
                onClick={() => chooseRange(value)}
              >
                {t(label)}
              </Button>
            ))}
          </Space>
          <Space wrap>
            <Text type="tertiary" size="small">
              {t("统计粒度")}
            </Text>
            <Select
              size="small"
              value={granularity}
              onChange={setGranularity}
              style={{ width: 110 }}
              optionList={[
                { value: "auto", label: t("自动") },
                { value: "5m", label: t("5 分钟") },
              ]}
            />
            <Button
              size="small"
              icon={<SlidersHorizontal size={15} />}
              onClick={() => setAdvancedOpen((value) => !value)}
            >
              {t("高级筛选")}
              <ChevronDown
                size={14}
                style={{
                  transform: advancedOpen ? "rotate(180deg)" : "none",
                }}
              />
            </Button>
            <Button
              size="small"
              theme="borderless"
              type="tertiary"
              icon={<RotateCcw size={15} />}
              onClick={resetFilters}
            >
              {t("重置")}
            </Button>
          </Space>
        </div>

        {range === "custom" && (
          <div className="mt-3 max-w-xl">
            <DatePicker
              type="dateTimeRange"
              value={customRange}
              onChange={changeCustomRange}
              style={{ width: "100%" }}
              placeholder={[t("开始时间"), t("结束时间")]}
            />
          </div>
        )}

        <Collapsible isOpen={advancedOpen} keepDOM>
          <div
            className="mt-3 grid grid-cols-1 gap-3 border-t pt-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5"
            style={{ borderColor: "var(--semi-color-border)" }}
          >
            <Select
              loading={filterLoading}
              showClear
              filter
              value={filters.group || undefined}
              placeholder={t("全部请求分组")}
              optionList={filterOptions.groups.map((item) => ({
                value: item.code,
                label:
                  item.name === item.code
                    ? item.name
                    : `${item.name} (${item.code})`,
              }))}
              onChange={(value) => updateFilter("group", value || "")}
            />
            <Select
              loading={filterLoading}
              showClear
              filter
              value={filters.channelId || undefined}
              placeholder={t("全部渠道")}
              optionList={filterOptions.channels.map((item) => ({
                value: String(item.channel_id),
                label: `${item.channel_name} (#${item.channel_id})`,
              }))}
              onChange={(value) => updateFilter("channelId", value || "")}
            />
            <Select
              loading={filterLoading}
              showClear
              value={filters.channelType || undefined}
              placeholder={t("全部渠道类型")}
              optionList={filterOptions.channelTypes.map((item) => ({
                value: String(item.value),
                label: item.label,
              }))}
              onChange={(value) => updateFilter("channelType", value || "")}
            />
            <Select
              remote
              showClear
              filter
              value={filters.requestedModelValue || undefined}
              placeholder={t("全部请求模型")}
              optionList={filterOptions.requestedModels}
              onSearch={(value) => searchModels("requested", value)}
              onChange={(value) => changeModel("requested", value)}
            />
            <Select
              remote
              showClear
              filter
              value={filters.upstreamModelValue || undefined}
              placeholder={t("全部上游模型")}
              optionList={filterOptions.upstreamModels}
              onSearch={(value) => searchModels("upstream", value)}
              onChange={(value) => changeModel("upstream", value)}
            />
            <Select
              showClear
              value={filters.outcome || undefined}
              placeholder={t("全部结果")}
              optionList={(filterOptions.outcomes.length
                ? filterOptions.outcomes
                : Object.keys(OUTCOME_LABELS)
              ).map((value) => ({
                value,
                label: t(OUTCOME_LABELS[value] || value),
              }))}
              onChange={(value) => updateFilter("outcome", value || "")}
            />
            <Input
              value={filters.statusCode}
              showClear
              placeholder={t("状态码，如 429 或 5xx")}
              onChange={(value) => updateFilter("statusCode", value.trim())}
            />
            <Select
              value={filters.stream}
              placeholder={t("响应方式")}
              optionList={[
                { value: "", label: t("全部响应方式") },
                { value: "true", label: t("流式") },
                { value: "false", label: t("非流式") },
              ]}
              onChange={(value) => updateFilter("stream", value)}
            />
            <Select
              value={filters.dataOrigin}
              placeholder={t("数据来源")}
              optionList={[
                { value: "live,legacy", label: t("实时 + 历史") },
                { value: "live", label: t("仅实时精确采集") },
                { value: "legacy", label: t("仅历史日志推导") },
              ]}
              onChange={(value) => updateFilter("dataOrigin", value)}
            />
            <Select
              value={filters.trafficSource}
              placeholder={t("流量来源")}
              optionList={[
                { value: "relay", label: t("真实转发") },
                { value: "playground", label: t("后台调试") },
                { value: "task", label: t("异步任务") },
              ]}
              onChange={(value) => updateFilter("trafficSource", value)}
            />
          </div>
        </Collapsible>
        <div className="mt-2">
          <Text type="tertiary" size="small">
            {t(VIEW_HINTS[activeView])}
          </Text>
        </div>
      </Card>

      {qualityMessages.length > 0 && activeView !== "probe" && (
        <Banner
          className="mb-4"
          type={
            viewStatus.meta?.backfill?.status === "failed"
              ? "danger"
              : "warning"
          }
          description={t("数据质量提示：{{messages}}", {
            messages: new Intl.ListFormat(getFormattingLocale(), {
              style: "long",
              type: "conjunction",
            }).format(qualityMessages),
          })}
        />
      )}

      {activeView === "overview" && (
        <OverviewView {...commonViewProps} onViewChange={setActiveView} />
      )}
      {activeView === "operations" && (
        <OperationsView
          {...commonViewProps}
          retentionDays={retentionDays}
          preset={operationsPreset}
        />
      )}
      {activeView === "channels" && (
        <ChannelsView
          {...commonViewProps}
          onGroupChannels={handleGroupChannels}
          onShowFailures={handleShowFailures}
        />
      )}
      {activeView === "failures" && (
        <FailuresView
          {...commonViewProps}
          statusCode={filters.statusCode}
          onClearStatusCode={() => updateFilter("statusCode", "")}
        />
      )}
      {activeView === "probe" && (
        <ProbeView
          refreshKey={refreshKey}
          onStatus={handleViewStatus}
          channelId={filters.channelId}
          channelType={filters.channelType}
          requestedModel={filters.requestedModel}
        />
      )}
    </div>
  );
};

export default ChannelObservability;
