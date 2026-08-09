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

import React, { useEffect, useMemo, useState } from "react";
import {
  Banner,
  Button,
  ButtonGroup,
  Card,
  Empty,
  Pagination,
  Progress,
  Space,
  Spin,
  Tag,
  Typography,
} from "@douyinfe/semi-ui";
import { ExternalLink } from "lucide-react";
import { useTranslation } from "react-i18next";
import { API } from "../../helpers";
import {
  API_ROOT,
  FAILURE_OWNER_LABELS,
  OUTCOME_LABELS,
  PAGE_SIZE,
  STAGE_LABELS,
  formatDateTime,
  formatDuration,
  formatInteger,
  getErrorMessage,
  normalizeStatusItems,
  statusTagColor,
} from "./utils";

const { Text } = Typography;

const StatusDistribution = ({ items, emptyText }) => {
  if (!items.length) return <Empty description={emptyText} />;
  const maximum = Math.max(1, ...items.map((item) => item.count));
  return (
    <div className="space-y-3">
      {items.slice(0, 12).map((item, index) => (
        <div key={`${item.label}-${index}`}>
          <div className="mb-1 flex items-center justify-between gap-3">
            <Tag color={statusTagColor(item.code, item.present)}>
              {item.label}
            </Tag>
            <Text strong>{formatInteger(item.count)}</Text>
          </div>
          <Progress percent={(item.count / maximum) * 100} showInfo={false} />
        </div>
      ))}
    </div>
  );
};

const statusValue = (row, prefix) => {
  if (!row?.[`${prefix}_status_present`]) return null;
  const value = Number(row[`${prefix}_status_code`]);
  return Number.isFinite(value) ? value : null;
};

const safeRelativeUrl = (value) =>
  typeof value === "string" && value.startsWith("/") && !value.startsWith("//")
    ? value
    : "";

const FailuresView = ({
  makeParams,
  queryKey,
  refreshKey,
  onStatus,
  statusCode,
  onClearStatusCode,
}) => {
  const { t } = useTranslation();
  const [scope, setScope] = useState("upstream");
  const [page, setPage] = useState(1);
  const [statuses, setStatuses] = useState([]);
  const [stages, setStages] = useState([]);
  const [failures, setFailures] = useState([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => setPage(1), [queryKey, scope]);

  useEffect(() => {
    const controller = new AbortController();
    const load = async () => {
      setLoading(true);
      setError("");
      onStatus?.({ loading: true, error: "", meta: null });
      try {
        const statusParams = makeParams(
          {
            metric_scope:
              scope === "client" ? "final_request" : "upstream_call",
          },
          { statusScope: scope },
        );
        const failureParams = makeParams(
          {
            page,
            page_size: PAGE_SIZE,
            sort_by: "created_at",
            sort_order: "desc",
          },
          { statusScope: scope, includeStream: false },
        );
        const [statusRes, failureRes] = await Promise.all([
          API.get(`${API_ROOT}/status-codes`, {
            params: statusParams,
            signal: controller.signal,
            skipErrorHandler: true,
          }),
          API.get(`${API_ROOT}/failures`, {
            params: failureParams,
            signal: controller.signal,
            skipErrorHandler: true,
          }),
        ]);
        const statusPayload = statusRes.data?.data || {};
        const failurePayload = failureRes.data?.data || {};
        const statusRows = normalizeStatusItems(statusPayload, t);
        const stageRows = (statusPayload.error_stages || [])
          .map((row) => ({
            code: -1,
            present: true,
            label:
              STAGE_LABELS[row.error_stage] || row.error_stage || t("未知阶段"),
            count: Number(row.count || 0),
          }))
          .sort((left, right) => right.count - left.count);
        const failureRows = failurePayload.items || [];
        setStatuses(statusRows);
        setStages(stageRows);
        setFailures(failureRows);
        setTotal(Number(failurePayload.total || failureRows.length));
        onStatus?.({
          loading: false,
          error: "",
          meta: statusPayload.meta || failurePayload.meta,
          empty:
            statusRows.reduce((sum, item) => sum + item.count, 0) === 0 &&
            failureRows.length === 0,
        });
      } catch (requestError) {
        if (requestError?.name === "CanceledError" || controller.signal.aborted)
          return;
        const message = getErrorMessage(requestError, t("失败分析加载失败"));
        setError(message);
        onStatus?.({ loading: false, error: message, meta: null });
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
    };
    load();
    return () => controller.abort();
  }, [makeParams, onStatus, page, queryKey, refreshKey, scope, t]);

  const groups = useMemo(() => {
    const result = {
      success: 0,
      client: 0,
      server: 0,
      noResponse: 0,
      unknown: 0,
    };
    statuses.forEach((item) => {
      if (!item.present) result.unknown += item.count;
      else if (item.code === 0) result.noResponse += item.count;
      else if (item.code >= 200 && item.code < 300)
        result.success += item.count;
      else if (item.code >= 400 && item.code < 500) result.client += item.count;
      else if (item.code >= 500) result.server += item.count;
      else result.unknown += item.count;
    });
    return result;
  }, [statuses]);

  const changeScope = (nextScope) => {
    if (nextScope === scope) return;
    if (nextScope === "client" && String(statusCode).trim() === "0") {
      onClearStatusCode?.();
    }
    setScope(nextScope);
    setPage(1);
  };

  if (error) return <Banner type="danger" description={error} />;

  return (
    <Spin spinning={loading}>
      <div className="space-y-4">
        <Card title={t("失败概览")}>
          <div className="mb-5 flex flex-wrap items-center justify-between gap-3 border-b pb-3">
            <Text type="tertiary" size="small">
              {scope === "client"
                ? t("最终客户端状态码，按逻辑请求统计")
                : t("上游原始状态码，按底层调用统计")}
            </Text>
            <ButtonGroup>
              <Button
                size="small"
                type={scope === "upstream" ? "primary" : "tertiary"}
                theme={scope === "upstream" ? "solid" : "light"}
                onClick={() => changeScope("upstream")}
              >
                {t("上游")}
              </Button>
              <Button
                size="small"
                type={scope === "client" ? "primary" : "tertiary"}
                theme={scope === "client" ? "solid" : "light"}
                onClick={() => changeScope("client")}
              >
                {t("客户端")}
              </Button>
            </ButtonGroup>
          </div>
          <div className="grid grid-cols-1 gap-6 xl:grid-cols-2">
            <section className="min-w-0">
              <Text strong>{t("状态码分布")}</Text>
              <Space wrap className="my-4">
                <Tag color="green">2xx · {formatInteger(groups.success)}</Tag>
                <Tag color="amber">4xx · {formatInteger(groups.client)}</Tag>
                <Tag color="red">5xx · {formatInteger(groups.server)}</Tag>
                <Tag color="red">
                  {t("无响应")} · {formatInteger(groups.noResponse)}
                </Tag>
                <Tag color="grey">
                  {t("未知")} · {formatInteger(groups.unknown)}
                </Tag>
              </Space>
              <StatusDistribution
                items={statuses}
                emptyText={t("暂无状态码数据")}
              />
            </section>

            <section className="min-w-0 border-t pt-5 xl:border-l xl:border-t-0 xl:pl-6 xl:pt-0">
              <Text strong>{t("错误阶段")}</Text>
              <Text type="tertiary" size="small" className="mt-1 block">
                {t("快速识别失败发生在请求链路的哪个环节")}
              </Text>
              <div className="mt-4">
                <StatusDistribution
                  items={stages}
                  emptyText={t("暂无错误阶段数据")}
                />
              </div>
            </section>
          </div>
        </Card>

        <Card
          title={t("最近失败请求")}
          headerExtraContent={
            <Text type="tertiary">
              {t("共 {{count}} 条", { count: formatInteger(total) })}
            </Text>
          }
        >
          {!failures.length ? (
            <Empty description={t("当前范围没有近期失败明细")} />
          ) : (
            <div>
              {failures.map((failure, index) => {
                const selectedCode =
                  scope === "client"
                    ? statusValue(failure, "client")
                    : statusValue(failure, "upstream");
                const statusLabel =
                  selectedCode === null
                    ? t("状态未知")
                    : selectedCode === 0
                      ? t("无响应")
                      : String(selectedCode);
                const logUrl = safeRelativeUrl(failure.log_url);
                return (
                  <div
                    key={failure.event_id || `${failure.request_id}-${index}`}
                    className="py-4 first:pt-0 last:pb-0"
                    style={{
                      borderBottom:
                        index === failures.length - 1
                          ? "none"
                          : "1px solid var(--semi-color-border)",
                    }}
                  >
                    <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
                      <Space wrap>
                        <Tag
                          color={statusTagColor(
                            selectedCode ?? -1,
                            selectedCode !== null,
                          )}
                        >
                          {statusLabel}
                        </Tag>
                        <Text strong>
                          {failure.channel_name || t("未知渠道")} ·{" "}
                          {failure.requested_model ||
                            failure.upstream_model ||
                            t("未知模型")}
                        </Text>
                      </Space>
                      <Text type="tertiary" size="small">
                        {formatDateTime(failure.created_at)}
                      </Text>
                    </div>
                    <div className="my-3">
                      <Text>
                        {failure.error_summary || t("未提供错误摘要")}
                      </Text>
                    </div>
                    <Space wrap spacing="tight">
                      {failure.request_id && (
                        <Tag>
                          {t("请求 ID")} {failure.request_id}
                        </Tag>
                      )}
                      <Tag>
                        {t(OUTCOME_LABELS[failure.outcome] || failure.outcome)}
                      </Tag>
                      <Tag>
                        {failure.data_origin === "legacy"
                          ? t("历史日志推导")
                          : t("实时精确采集")}
                      </Tag>
                      <Tag>
                        {t(
                          STAGE_LABELS[failure.error_stage] ||
                            failure.error_stage ||
                            "未知阶段",
                        )}
                      </Tag>
                      <Tag>
                        {t("尝试 #{{seq}}", {
                          seq: failure.attempt_seq || "-",
                        })}
                      </Tag>
                      <Tag>
                        {failure.retry_planned
                          ? t("已计划重试")
                          : t("未继续重试")}
                      </Tag>
                      <Tag>
                        {t("归因 {{owner}}", {
                          owner: t(
                            FAILURE_OWNER_LABELS[failure.failure_owner] ||
                              failure.failure_owner ||
                              "未知",
                          ),
                        })}
                      </Tag>
                      <Tag>
                        {t("耗时 {{duration}}", {
                          duration: formatDuration(failure.latency_ms),
                        })}
                      </Tag>
                      {failure.partial_response && (
                        <Tag color="amber">{t("已返回部分响应")}</Tag>
                      )}
                      {logUrl && (
                        <a href={logUrl} target="_top" rel="noreferrer">
                          <Tag
                            color="blue"
                            prefixIcon={<ExternalLink size={12} />}
                          >
                            {t("查看完整日志")}
                          </Tag>
                        </a>
                      )}
                    </Space>
                  </div>
                );
              })}
            </div>
          )}
          {total > PAGE_SIZE && (
            <div className="mt-4 flex justify-end">
              <Pagination
                currentPage={page}
                pageSize={PAGE_SIZE}
                total={total}
                onPageChange={setPage}
              />
            </div>
          )}
        </Card>
      </div>
    </Spin>
  );
};

export default FailuresView;
