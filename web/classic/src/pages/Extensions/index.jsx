import React, { useEffect, useMemo, useRef, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Card,
  Empty,
  Popconfirm,
  Space,
  Spin,
  Switch,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { ExternalLink, Puzzle, RefreshCw, Trash2, Upload } from 'lucide-react';
import { API, isRoot, showError, showSuccess } from '../../helpers';

const { Text, Title } = Typography;
const CLASSIC_EXTENSION_REFRESH_EVENT = 'classic-extension-refresh';

const notifyClassicSidebar = () => {
  window.dispatchEvent(new Event(CLASSIC_EXTENSION_REFRESH_EVENT));
};

const extensionPageUrl = (moduleId, pagePath) => {
  const normalizedPath = String(pagePath || '/').startsWith('/')
    ? pagePath
    : `/${pagePath}`;
  return `/api/extensions/${encodeURIComponent(moduleId)}/proxy${normalizedPath}`;
};

const readModules = (res) => res?.data?.data?.modules || [];
const readRoot = (res) => res?.data?.data?.root || 'data/modules';

const pageShellStyle = {
  padding: '64px 24px 24px',
};

export default function Extensions() {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [modules, setModules] = useState([]);
  const [rootDir, setRootDir] = useState('data/modules');
  const [pendingId, setPendingId] = useState('');
  const fileInputRef = useRef(null);

  const loadData = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/extension-admin/', {
        params: { all: 'true' },
      });
      if (res?.data?.success) {
        setModules(readModules(res));
        setRootDir(readRoot(res));
      } else {
        showError(res?.data?.message || t('扩展模块加载失败'));
      }
    } catch (error) {
      showError(error.message || t('扩展模块加载失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, []);

  const refreshModules = async () => {
    setRefreshing(true);
    try {
      const res = await API.post('/api/extension-admin/refresh');
      if (res?.data?.success) {
        setModules(readModules(res));
        setRootDir(readRoot(res));
        notifyClassicSidebar();
        showSuccess(t('扩展模块已刷新'));
      } else {
        showError(res?.data?.message || t('刷新失败'));
      }
    } catch (error) {
      showError(error.message || t('刷新失败'));
    } finally {
      setRefreshing(false);
    }
  };

  const uploadModule = async (event) => {
    const file = event.target.files?.[0];
    if (!file) return;
    if (!file.name.toLowerCase().endsWith('.zip')) {
      showError(t('请上传 zip 模块包'));
      event.target.value = '';
      return;
    }

    const formData = new FormData();
    formData.append('file', file);
    setUploading(true);
    try {
      const res = await API.post('/api/extension-admin/upload', formData);
      if (res?.data?.success) {
        showSuccess(t('模块已上传'));
        await loadData();
        notifyClassicSidebar();
      } else {
        showError(res?.data?.message || t('上传失败'));
      }
    } catch (error) {
      showError(error.message || t('上传失败'));
    } finally {
      setUploading(false);
      event.target.value = '';
    }
  };

  const setEnabled = async (module, enabled) => {
    setPendingId(module.id);
    try {
      const res = await API.put(
        `/api/extension-admin/${encodeURIComponent(module.id)}/enabled`,
        { enabled },
      );
      if (res?.data?.success) {
        showSuccess(t('扩展模块已更新'));
        await loadData();
        notifyClassicSidebar();
      } else {
        showError(res?.data?.message || t('更新失败'));
      }
    } catch (error) {
      showError(error.message || t('更新失败'));
    } finally {
      setPendingId('');
    }
  };

  const uninstallModule = async (module) => {
    setPendingId(module.id);
    try {
      const res = await API.delete(
        `/api/extension-admin/${encodeURIComponent(module.id)}`,
      );
      if (res?.data?.success) {
        showSuccess(t('模块已卸载'));
        await loadData();
        notifyClassicSidebar();
      } else {
        showError(res?.data?.message || t('卸载失败'));
      }
    } catch (error) {
      showError(error.message || t('卸载失败'));
    } finally {
      setPendingId('');
    }
  };

  const columns = useMemo(
    () => [
      {
        title: t('模块'),
        dataIndex: 'name',
        render: (name, record) => (
          <div style={{ display: 'flex', gap: 12, alignItems: 'flex-start' }}>
            <div
              style={{
                width: 36,
                height: 36,
                borderRadius: 8,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                color: 'var(--semi-color-text-2)',
                background: 'var(--semi-color-fill-0)',
                flexShrink: 0,
              }}
            >
              <Puzzle size={16} />
            </div>
            <div style={{ minWidth: 0 }}>
              <Space spacing={6} wrap>
                <Text strong>{name}</Text>
                <Tag color='blue' size='small'>
                  {record.version}
                </Tag>
                {record.error ? (
                  <Tag color='red' size='small'>
                    {t('无效')}
                  </Tag>
                ) : null}
              </Space>
              <div>
                <Text type='tertiary' size='small'>
                  {record.id}
                </Text>
              </div>
              {record.description ? (
                <div style={{ marginTop: 4 }}>
                  <Text type='secondary' size='small'>
                    {record.description}
                  </Text>
                </div>
              ) : null}
              {record.error ? (
                <div style={{ marginTop: 4 }}>
                  <Text type='danger' size='small'>
                    {record.error}
                  </Text>
                </div>
              ) : null}
            </div>
          </div>
        ),
      },
      {
        title: t('权限'),
        dataIndex: 'permissions',
        width: 180,
        render: (permissions) => {
          const roles = permissions?.roles?.length ? permissions.roles : ['user'];
          return (
            <Space spacing={4} wrap>
              {roles.map((role) => (
                <Tag key={role} size='small'>
                  {role}
                </Tag>
              ))}
            </Space>
          );
        },
      },
      {
        title: t('页面'),
        width: 180,
        render: (_, record) => {
          const firstPage = record.ui?.pages?.[0];
          if (!firstPage) {
            return <Text type='tertiary'>-</Text>;
          }
          return (
            <Link
              to={`/console/extensions/${encodeURIComponent(record.id)}/${encodeURIComponent(firstPage.key)}`}
              style={{ textDecoration: 'none' }}
            >
              <Button
                size='small'
                theme='outline'
                disabled={!record.enabled}
                icon={<ExternalLink size={14} />}
              >
                {firstPage.title || t('打开')}
              </Button>
            </Link>
          );
        },
      },
      {
        title: t('启用'),
        width: 100,
        render: (_, record) => (
          <Switch
            checked={record.enabled}
            disabled={Boolean(record.error) || pendingId === record.id}
            loading={pendingId === record.id}
            onChange={(checked) => setEnabled(record, checked)}
          />
        ),
      },
      {
        title: t('操作'),
        width: 120,
        align: 'right',
        render: (_, record) => (
          <Popconfirm
            title={t('确认卸载模块？')}
            content={t('这将删除模块文件并移除启用状态，此操作不可撤销。')}
            okText={t('卸载')}
            cancelText={t('取消')}
            onConfirm={() => uninstallModule(record)}
          >
            <Button
              size='small'
              type='danger'
              theme='outline'
              disabled={pendingId === record.id}
              loading={pendingId === record.id}
              icon={<Trash2 size={14} />}
            >
              {t('卸载')}
            </Button>
          </Popconfirm>
        ),
      },
    ],
    [pendingId, t],
  );

  if (!isRoot()) {
    return (
      <div style={pageShellStyle}>
        <Card>
          <Empty description={t('只有 root 用户可以管理扩展模块')} />
        </Card>
      </div>
    );
  }

  return (
    <div style={pageShellStyle}>
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          gap: 16,
          alignItems: 'center',
          flexWrap: 'wrap',
          marginBottom: 16,
        }}
      >
        <div>
          <Title heading={3} style={{ margin: 0 }}>
            {t('模块管理')}
          </Title>
          <Text type='secondary'>
            {t('上传 zip 模块包，或把模块放入模块目录，刷新后即可启用，无需重启主程序。')}
          </Text>
        </div>
        <Space wrap>
          <input
            ref={fileInputRef}
            type='file'
            accept='.zip,application/zip'
            style={{ display: 'none' }}
            onChange={uploadModule}
          />
          <Button
            theme='outline'
            loading={uploading}
            icon={<Upload size={16} />}
            onClick={() => fileInputRef.current?.click()}
          >
            {t('上传模块')}
          </Button>
          <Button
            theme='outline'
            loading={refreshing}
            icon={<RefreshCw size={16} />}
            onClick={refreshModules}
          >
            {t('刷新')}
          </Button>
        </Space>
      </div>

      <Card
        title={t('模块目录')}
        headerExtraContent={
          <Text type='tertiary' style={{ wordBreak: 'break-all' }}>
            {rootDir}
          </Text>
        }
      >
        <Spin spinning={loading}>
          {modules.length === 0 && !loading ? (
            <Empty description={t('未发现扩展模块')} />
          ) : (
            <Table
              rowKey='id'
              columns={columns}
              dataSource={modules}
              pagination={false}
            />
          )}
        </Spin>
      </Card>
    </div>
  );
}

export function ExtensionModulePage() {
  const { t } = useTranslation();
  const { moduleId, pageKey } = useParams();
  const [loading, setLoading] = useState(true);
  const [module, setModule] = useState(null);
  const [page, setPage] = useState(null);

  useEffect(() => {
    let cancelled = false;
    const loadPage = async () => {
      setLoading(true);
      try {
        const res = await API.get('/api/extensions/');
        if (!res?.data?.success) {
          showError(res?.data?.message || t('扩展模块加载失败'));
          return;
        }
        const modules = readModules(res);
        const foundModule = modules.find((item) => item.id === moduleId);
        const foundPage = foundModule?.ui?.pages?.find(
          (item) => item.key === pageKey,
        );
        if (!cancelled) {
          setModule(foundModule || null);
          setPage(foundPage || null);
        }
      } catch (error) {
        showError(error.message || t('扩展模块加载失败'));
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };
    loadPage();
    return () => {
      cancelled = true;
    };
  }, [moduleId, pageKey, t]);

  if (loading) {
    return (
      <div style={{ padding: 24 }}>
        <Spin spinning />
      </div>
    );
  }

  if (!module || !page) {
    return (
      <div style={{ padding: 24 }}>
        <Card>
          <Empty description={t('扩展页面不存在，请刷新扩展模块或检查 manifest')} />
        </Card>
      </div>
    );
  }

  const src = extensionPageUrl(module.id, page.path);

  if (page.embed === false) {
    return (
      <div style={{ padding: 24 }}>
        <Card>
          <Empty
            title={page.title || module.name}
            description={t('该扩展页面配置为外部打开')}
          >
            <Button
              type='primary'
              icon={<ExternalLink size={16} />}
              onClick={() => window.open(src, '_blank', 'noopener,noreferrer')}
            >
              {t('打开扩展')}
            </Button>
          </Empty>
        </Card>
      </div>
    );
  }

  return (
    <div
      style={{
        height: 'calc(100vh - 64px)',
        display: 'flex',
        flexDirection: 'column',
        background: 'var(--semi-color-bg-0)',
      }}
    >
      <div
        style={{
          height: 56,
          padding: '10px 16px',
          borderBottom: '1px solid var(--semi-color-border)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 16,
          flexShrink: 0,
        }}
      >
        <div style={{ minWidth: 0 }}>
          <Text strong ellipsis>
            {page.title || module.name}
          </Text>
          <div>
            <Text type='tertiary' size='small' ellipsis>
              {module.name} · {module.version}
            </Text>
          </div>
        </div>
        <Button
          size='small'
          theme='outline'
          icon={<ExternalLink size={14} />}
          onClick={() => window.open(src, '_blank', 'noopener,noreferrer')}
        >
          {t('打开')}
        </Button>
      </div>
      <iframe
        src={src}
        title={page.title || module.name}
        style={{
          flex: 1,
          width: '100%',
          minHeight: 0,
          border: 0,
          background: 'var(--semi-color-bg-0)',
        }}
      />
    </div>
  );
}
