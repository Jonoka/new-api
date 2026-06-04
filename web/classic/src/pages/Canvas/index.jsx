import React, { useContext, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Card, Select, Typography } from '@douyinfe/semi-ui';
import { ArrowUpRight, Brush } from 'lucide-react';
import {
  API,
  buildCanvasLaunchUrl,
  CANVAS_APP_ORIGIN,
  processGroupsData,
  showError,
} from '../../helpers';
import { UserContext } from '../../context/User';
import { API_ENDPOINTS } from '../../constants/playground.constants';

const Canvas = () => {
  const { t } = useTranslation();
  const [userState] = useContext(UserContext);
  const [groups, setGroups] = useState([]);
  const [selectedGroup, setSelectedGroup] = useState('');
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    const loadGroups = async () => {
      setLoading(true);
      try {
        const res = await API.get(API_ENDPOINTS.USER_GROUPS);
        const { success, message, data } = res.data;
        if (!success) {
          showError(t(message));
          return;
        }

        const userGroup =
          userState?.user?.group ||
          JSON.parse(localStorage.getItem('user') || '{}')?.group;
        const groupOptions = processGroupsData(data, userGroup);
        setGroups(groupOptions);

        const fallback =
          groupOptions.find((group) => group.value === 'default')?.value ||
          groupOptions[0]?.value ||
          '';
        setSelectedGroup((current) => current || fallback);
      } catch (error) {
        showError(t('加载分组失败'));
      } finally {
        setLoading(false);
      }
    };

    loadGroups();
  }, [t, userState?.user?.group]);

  const launchUrl = useMemo(() => {
    if (!selectedGroup || typeof window === 'undefined') return '';

    return buildCanvasLaunchUrl({
      canvasOrigin: CANVAS_APP_ORIGIN,
      newApiOrigin: window.location.origin,
      group: selectedGroup,
    });
  }, [selectedGroup]);

  const openCanvas = () => {
    if (!launchUrl) return;
    window.open(launchUrl, '_blank', 'noopener');
  };

  return (
    <div className='flex min-h-[calc(100vh-64px)] items-center justify-center p-4 md:p-8'>
      <Card className='w-full max-w-xl !rounded-2xl shadow-sm'>
        <div className='mb-6 flex h-11 w-11 items-center justify-center rounded-xl bg-blue-50 text-blue-600'>
          <Brush size={22} />
        </div>

        <Typography.Title heading={3} className='!mb-2'>
          {t('无限画布')}
        </Typography.Title>
        <Typography.Text type='secondary' className='block !mb-6'>
          {t('选择分组后打开无限画布，画布会使用当前登录态调用模型。')}
        </Typography.Text>

        <div className='mb-5'>
          <div className='mb-2 text-sm font-medium text-gray-900'>
            {t('模型分组')}
          </div>
          <Select
            value={selectedGroup}
            onChange={setSelectedGroup}
            optionList={groups}
            loading={loading}
            disabled={loading || groups.length === 0}
            filter
            style={{ width: '100%' }}
          />
        </div>

        <Button
          type='primary'
          block
          icon={<ArrowUpRight size={16} />}
          onClick={openCanvas}
          disabled={!launchUrl || loading}
        >
          {t('打开无限画布')}
        </Button>
      </Card>
    </div>
  );
};

export default Canvas;
