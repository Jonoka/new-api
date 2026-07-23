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

import React, { useEffect, useMemo, useState } from 'react';
import {
  Button,
  Card,
  Empty,
  Space,
  Spin,
  Typography,
} from '@douyinfe/semi-ui';
import { RefreshCw, TriangleAlert } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import './native-sdk';

const { Text } = Typography;

const nativeResourceUrl = (
  moduleId,
  pageKey,
  resource,
  moduleVersion,
  assetRevision,
  loadAttempt,
) => {
  const query = new URLSearchParams({
    module_version: String(moduleVersion || ''),
  });
  if (assetRevision) {
    query.set('asset_revision', String(assetRevision));
  }
  if (loadAttempt > 0) {
    query.set('load_attempt', String(loadAttempt));
  }
  return `/api/extensions/${encodeURIComponent(moduleId)}/native/${encodeURIComponent(pageKey)}/classic/${resource}?${query.toString()}`;
};

const loadStylesheet = (url, owner, t) => {
  const link = document.createElement('link');
  const loaded = new Promise((resolve, reject) => {
    link.onload = resolve;
    link.onerror = () => {
      link.remove();
      reject(new Error(t('原生扩展样式加载失败：{{url}}', { url })));
    };
  });
  link.rel = 'stylesheet';
  link.href = url;
  link.dataset.newApiExtensionNative = owner;
  document.head.appendChild(link);
  return { link, loaded };
};

const errorMessage = (error, fallback) =>
  error instanceof Error ? error.message : String(error || fallback);

class NativeExtensionErrorBoundary extends React.Component {
  constructor(props) {
    super(props);
    this.state = { error: null };
  }

  static getDerivedStateFromError(error) {
    return { error };
  }

  render() {
    if (this.state.error) {
      return this.props.renderError(this.state.error);
    }
    return this.props.children;
  }
}

const NativeError = ({ error, onRetry }) => {
  const { t } = useTranslation();

  return (
    <div style={{ padding: 24 }}>
      <Card>
        <Empty
          image={<TriangleAlert size={48} />}
          title={t('原生扩展加载失败')}
          description={
            <Space vertical align='center' spacing={12}>
              <Text type='secondary'>{errorMessage(error, t('未知错误'))}</Text>
              <Button
                type='primary'
                icon={<RefreshCw size={16} />}
                onClick={onRetry}
              >
                {t('重试')}
              </Button>
            </Space>
          }
        />
      </Card>
    </div>
  );
};

export default function NativeExtensionHost({ module, page }) {
  const { t } = useTranslation();
  const [loadAttempt, setLoadAttempt] = useState(0);
  const [state, setState] = useState({ status: 'loading' });
  const target = page?.render?.targets?.classic;
  const owner = `${module?.id || 'unknown'}:${page?.key || 'unknown'}`;

  const configurationError = useMemo(() => {
    if (page?.render?.sdk !== 'v1') {
      return new Error(t('当前 Classic 前端不支持该扩展使用的原生 SDK 版本'));
    }
    if (!target || typeof target !== 'object') {
      return new Error(t('该扩展没有提供 Classic 原生页面资源'));
    }
    if (typeof target.entry !== 'string' || !target.entry.trim()) {
      return new Error(t('Classic 原生页面缺少入口资源'));
    }
    if (target.styles !== undefined && !Array.isArray(target.styles)) {
      return new Error(t('Classic 原生页面样式配置无效'));
    }
    return null;
  }, [page?.render?.sdk, t, target]);

  useEffect(() => {
    let cancelled = false;
    const links = [];

    if (configurationError) {
      setState({ status: 'error', error: configurationError });
      return undefined;
    }

    setState({ status: 'loading' });

    const load = async () => {
      try {
        for (let index = 0; index < (target.styles?.length || 0); index += 1) {
          const { link, loaded } = loadStylesheet(
            nativeResourceUrl(
              module.id,
              page.key,
              `style-${index}`,
              module.version,
              module.asset_revision,
              loadAttempt,
            ),
            owner,
            t,
          );
          links.push(link);
          await loaded;
          if (cancelled) {
            link.remove();
            return;
          }
        }

        const entryUrl = nativeResourceUrl(
          module.id,
          page.key,
          'entry',
          module.version,
          module.asset_revision,
          loadAttempt,
        );
        const loadedModule = await import(/* @vite-ignore */ entryUrl);
        const Component = loadedModule?.default;
        if (!Component || !['function', 'object'].includes(typeof Component)) {
          throw new Error(t('原生扩展入口必须默认导出 React 组件'));
        }
        if (!cancelled) {
          setState({ status: 'ready', Component });
        }
      } catch (error) {
        links.splice(0).forEach((link) => link.remove());
        if (!cancelled) {
          setState({ status: 'error', error });
        }
      }
    };

    load();

    return () => {
      cancelled = true;
      links.forEach((link) => link.remove());
    };
  }, [configurationError, loadAttempt, module, owner, page, t, target]);

  const retry = () => {
    setState({ status: 'loading' });
    setLoadAttempt((attempt) => attempt + 1);
  };

  if (state.status === 'loading') {
    return (
      <div
        style={{
          minHeight: 240,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        <Space vertical align='center'>
          <Spin spinning size='large' />
          <Text type='secondary'>{t('正在加载原生扩展')}</Text>
        </Space>
      </div>
    );
  }

  if (state.status === 'error') {
    return <NativeError error={state.error} onRetry={retry} />;
  }

  const Component = state.Component;
  const boundaryResetKey = [
    module?.id,
    module?.version,
    module?.asset_revision,
    page?.key,
    loadAttempt,
  ].join(':');
  return (
    <NativeExtensionErrorBoundary
      key={boundaryResetKey}
      renderError={(error) => <NativeError error={error} onRetry={retry} />}
    >
      <div
        data-extension-native-host={owner}
        style={{ width: '100%', minWidth: 0 }}
      >
        <Component module={module} page={page} platform='classic' />
      </div>
    </NativeExtensionErrorBoundary>
  );
}
