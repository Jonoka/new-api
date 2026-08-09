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
  Card,
  Empty,
  Input,
  Select,
  Table,
  Tag,
  Toast,
  Typography,
} from "@douyinfe/semi-ui";
import { Play, Search } from "lucide-react";
import { useTranslation } from "react-i18next";
import { API } from "../../helpers";
import {
  formatChannelTypeName,
  formatDuration,
  formatRelativeTime,
  getFormattingLocale,
  getErrorMessage,
  normalizeTimestamp,
} from "./utils";

const { Text } = Typography;

const normalizeProbeChannel = (row, t) => {
  const id = Number(row.id ?? row.channel_id ?? 0);
  const type = Number(row.type ?? row.channel_type ?? 0);
  const rawModels = row.models || [];
  const models = Array.isArray(rawModels)
    ? rawModels.map(String).filter(Boolean)
    : String(rawModels)
        .split(",")
        .map((item) => item.trim())
        .filter(Boolean);
  return {
    id,
    name: String(row.name || row.channel_name || t("渠道 #{{id}}", { id })),
    type,
    typeName: formatChannelTypeName(type, t),
    status: Number(row.status || 0),
    models,
    responseTime: Number(row.response_time ?? Number.NaN),
    testTime: normalizeTimestamp(row.test_time),
  };
};

const probeStatus = (status, t) => {
  if (status === 1) return { label: t("已启用"), color: "green" };
  if (status === 2) return { label: t("手动禁用"), color: "amber" };
  if (status === 3) return { label: t("自动禁用"), color: "red" };
  return { label: t("已禁用"), color: "grey" };
};

const ProbeView = ({
  refreshKey,
  onStatus,
  channelId,
  channelType,
  requestedModel,
}) => {
  const { t } = useTranslation();
  const [channels, setChannels] = useState([]);
  const [keyword, setKeyword] = useState("");
  const [selectedModels, setSelectedModels] = useState(new Map());
  const [results, setResults] = useState(new Map());
  const [testing, setTesting] = useState(new Set());
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    const controller = new AbortController();
    const load = async () => {
      setLoading(true);
      setError("");
      onStatus?.({ loading: true, error: "", meta: null });
      try {
        const all = [];
        let page = 1;
        let total = 0;
        do {
          const response = await API.get("/api/channel/search", {
            params: {
              keyword: "",
              group: "",
              model: "",
              id_sort: true,
              tag_mode: false,
              p: page,
              page_size: 100,
            },
            signal: controller.signal,
            skipErrorHandler: true,
          });
          const payload = response.data?.data || {};
          const rows = payload.items || payload.channels || [];
          total = Number(payload.total || rows.length);
          all.push(...rows);
          page += 1;
          if (!rows.length) break;
        } while (all.length < total && page <= 101);
        const normalized = all.map((row) => normalizeProbeChannel(row, t));
        setChannels(normalized);
        setSelectedModels((current) => {
          const next = new Map(current);
          normalized.forEach((channel) => {
            if (!next.has(channel.id) && channel.models[0]) {
              next.set(channel.id, channel.models[0]);
            }
          });
          return next;
        });
        onStatus?.({
          loading: false,
          error: "",
          meta: { generated_at: Math.floor(Date.now() / 1000) },
          empty: false,
        });
      } catch (requestError) {
        if (requestError?.name === "CanceledError" || controller.signal.aborted)
          return;
        const message = getErrorMessage(requestError, t("渠道列表加载失败"));
        setError(message);
        onStatus?.({ loading: false, error: message, meta: null });
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
    };
    load();
    return () => controller.abort();
  }, [onStatus, refreshKey, t]);

  const filtered = useMemo(() => {
    const locale = getFormattingLocale();
    const normalizedKeyword = keyword.trim().toLocaleLowerCase(locale);
    return channels.filter((channel) => {
      if (channelId && String(channel.id) !== String(channelId)) return false;
      if (channelType && String(channel.type) !== String(channelType))
        return false;
      if (requestedModel && !channel.models.includes(requestedModel))
        return false;
      if (!normalizedKeyword) return true;
      return `${channel.name} ${channel.id} ${channel.typeName}`
        .toLocaleLowerCase(locale)
        .includes(normalizedKeyword);
    });
  }, [channelId, channelType, channels, keyword, requestedModel]);

  const testChannel = async (channelIdValue) => {
    const model = selectedModels.get(channelIdValue);
    if (!model || testing.has(channelIdValue)) return;
    setTesting((current) => new Set(current).add(channelIdValue));
    setResults((current) => {
      const next = new Map(current);
      next.delete(channelIdValue);
      return next;
    });
    const startedAt = performance.now();
    try {
      const response = await API.get(
        `/api/channel/test/${encodeURIComponent(channelIdValue)}`,
        { params: { model }, skipErrorHandler: true },
      );
      if (response.data?.success === false) {
        throw new Error(response.data?.message || t("探测失败"));
      }
      const payload = response.data?.data || {};
      const duration = Number.isFinite(Number(payload.time))
        ? Number(payload.time) * 1000
        : performance.now() - startedAt;
      setResults((current) =>
        new Map(current).set(channelIdValue, { success: true, duration }),
      );
      Toast.success({
        content: t("渠道 #{{id}} 探测成功，耗时 {{duration}}", {
          id: channelIdValue,
          duration: formatDuration(duration),
        }),
      });
    } catch (requestError) {
      const message = getErrorMessage(requestError, t("探测失败"));
      setResults((current) =>
        new Map(current).set(channelIdValue, { success: false, message }),
      );
      Toast.error({
        content: t("渠道 #{{id}} 探测失败：{{message}}", {
          id: channelIdValue,
          message,
        }),
      });
    } finally {
      setTesting((current) => {
        const next = new Set(current);
        next.delete(channelIdValue);
        return next;
      });
    }
  };

  const columns = [
    {
      title: t("渠道"),
      width: 240,
      render: (_, record) => (
        <div>
          <Text strong>{record.name}</Text>
          <div>
            <Text type="tertiary" size="small">
              #{record.id}
            </Text>
          </div>
        </div>
      ),
    },
    {
      title: t("状态"),
      width: 110,
      render: (_, record) => {
        const status = probeStatus(record.status, t);
        return <Tag color={status.color}>{status.label}</Tag>;
      },
    },
    { title: t("类型"), dataIndex: "typeName", width: 130 },
    {
      title: t("测试模型"),
      width: 260,
      render: (_, record) => (
        <Select
          value={selectedModels.get(record.id) || record.models[0] || ""}
          disabled={!record.models.length}
          style={{ width: "100%" }}
          optionList={record.models.map((model) => ({
            value: model,
            label: model,
          }))}
          onChange={(value) =>
            setSelectedModels((current) =>
              new Map(current).set(record.id, value),
            )
          }
        />
      ),
    },
    {
      title: t("最近测试"),
      width: 120,
      render: (_, record) => formatRelativeTime(record.testTime),
    },
    {
      title: t("耗时 / 结果"),
      width: 240,
      render: (_, record) => {
        const result = results.get(record.id);
        if (result?.success) {
          return <Tag color="green">{formatDuration(result.duration)}</Tag>;
        }
        if (result) {
          return (
            <Text type="danger" ellipsis={{ showTooltip: true }}>
              {result.message}
            </Text>
          );
        }
        return Number.isFinite(record.responseTime) ? (
          formatDuration(record.responseTime)
        ) : (
          <Text type="tertiary">{t("未测试")}</Text>
        );
      },
    },
    {
      title: t("操作"),
      width: 130,
      fixed: "right",
      render: (_, record) => (
        <Button
          icon={<Play size={15} />}
          loading={testing.has(record.id)}
          disabled={!selectedModels.get(record.id) && !record.models[0]}
          onClick={() => testChannel(record.id)}
        >
          {t("运行测试")}
        </Button>
      ),
    },
  ];

  return (
    <div className="space-y-4">
      <Banner
        type="info"
        description={t(
          "主动探测只回答当前是否能连通，探测流量不会进入真实业务统计。",
        )}
      />
      <Card title={t("渠道主动探测")}>
        {error ? (
          <Banner type="danger" description={error} />
        ) : (
          <>
            <div className="mb-4 border-b pb-3">
              <Input
                prefix={<Search size={15} />}
                value={keyword}
                onChange={setKeyword}
                showClear
                placeholder={t("搜索渠道、ID 或类型")}
                style={{ width: "min(260px, 100%)" }}
              />
            </div>
            <Table
              size="small"
              columns={columns}
              dataSource={filtered}
              rowKey="id"
              loading={loading}
              pagination={false}
              scroll={{ x: 1230 }}
              empty={<Empty description={t("没有匹配的渠道")} />}
            />
          </>
        )}
      </Card>
    </div>
  );
};

export default ProbeView;
