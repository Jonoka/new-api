import React, { useContext, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Card, Select, Typography } from '@douyinfe/semi-ui';
import { ArrowUpRight } from 'lucide-react';
import {
  API,
  buildCanvasLaunchUrl,
  getCanvasSettingsFromSidebarModules,
  getCustomNavIconComponent,
  processGroupsData,
  showError,
} from '../../helpers';
import { UserContext } from '../../context/User';
import { StatusContext } from '../../context/Status';
import { API_ENDPOINTS } from '../../constants/playground.constants';

const capabilityGroups = [
  { key: 'textGroup', label: '文本分组' },
  { key: 'imageGroup', label: '生图分组' },
  { key: 'audioGroup', label: '音频分组' },
  { key: 'videoGroup', label: '视频分组' },
];

const groupStateAccessors = {
  textGroup: ['textGroup', 'setTextGroup'],
  imageGroup: ['imageGroup', 'setImageGroup'],
  audioGroup: ['audioGroup', 'setAudioGroup'],
  videoGroup: ['videoGroup', 'setVideoGroup'],
};

const Canvas = () => {
  const { t } = useTranslation();
  const [userState] = useContext(UserContext);
  const [statusState] = useContext(StatusContext);
  const [groups, setGroups] = useState([]);
  const [defaultGroup, setDefaultGroup] = useState('');
  const [textGroup, setTextGroup] = useState('');
  const [imageGroup, setImageGroup] = useState('');
  const [audioGroup, setAudioGroup] = useState('');
  const [videoGroup, setVideoGroup] = useState('');
  const [loading, setLoading] = useState(false);
  const capabilityGroupState = {
    textGroup,
    setTextGroup,
    imageGroup,
    setImageGroup,
    audioGroup,
    setAudioGroup,
    videoGroup,
    setVideoGroup,
  };
  const canvasSettings = useMemo(
    () =>
      getCanvasSettingsFromSidebarModules(
        statusState?.status?.SidebarModulesAdmin,
      ),
    [statusState?.status?.SidebarModulesAdmin],
  );
  const CanvasIcon = getCustomNavIconComponent(canvasSettings.canvasIcon);

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
          groupOptions.find((group) => group.value === 'auto')?.value ||
          groupOptions.find((group) => group.value === 'default')?.value ||
          groupOptions[0]?.value ||
          '';
        setDefaultGroup((current) => current || fallback);
      } catch (error) {
        showError(t('加载分组失败'));
      } finally {
        setLoading(false);
      }
    };

    loadGroups();
  }, [t, userState?.user?.group]);

  const launchUrl = useMemo(() => {
    if (!defaultGroup || typeof window === 'undefined') return '';

    return buildCanvasLaunchUrl({
      canvasOrigin: canvasSettings.canvasOrigin,
      newApiOrigin: window.location.origin,
      group: defaultGroup,
      textGroup,
      imageGroup,
      audioGroup,
      videoGroup,
    });
  }, [
    audioGroup,
    canvasSettings.canvasOrigin,
    defaultGroup,
    imageGroup,
    textGroup,
    videoGroup,
  ]);

  const openCanvas = () => {
    if (!launchUrl) return;
    window.open(launchUrl, '_blank', 'noopener');
  };

  return (
    <div className='flex min-h-[calc(100vh-64px)] items-center justify-center p-4 md:p-8'>
      <Card className='w-full max-w-xl !rounded-2xl shadow-sm'>
        <div className='mb-6 flex h-11 w-11 items-center justify-center rounded-xl bg-blue-50 text-blue-600'>
          {CanvasIcon ? <CanvasIcon size={22} /> : null}
        </div>

        <Typography.Title heading={3} className='!mb-2'>
          {t('无限画布')}
        </Typography.Title>
        <Typography.Text type='secondary' className='block !mb-6'>
          {t('选择分组后打开无限画布，画布会使用当前登录态调用模型。')}
        </Typography.Text>

        <div className='mb-5'>
          <div className='mb-2 text-sm font-medium text-gray-900'>
            {t('默认分组')}
          </div>
          <Select
            value={defaultGroup}
            onChange={setDefaultGroup}
            optionList={groups}
            loading={loading}
            disabled={loading || groups.length === 0}
            filter
            style={{ width: '100%' }}
          />
        </div>

        <div className='mb-5 grid grid-cols-1 gap-3 md:grid-cols-2'>
          {capabilityGroups.map((item) => (
            <div key={item.key}>
              <div className='mb-2 text-sm font-medium text-gray-900'>
                {t(item.label)}
              </div>
              <Select
                value={capabilityGroupState[groupStateAccessors[item.key][0]]}
                onChange={
                  capabilityGroupState[groupStateAccessors[item.key][1]]
                }
                optionList={groups}
                loading={loading}
                disabled={loading || groups.length === 0}
                filter
                showClear
                onClear={() =>
                  capabilityGroupState[groupStateAccessors[item.key][1]]('')
                }
                placeholder={t('默认分组')}
                style={{ width: '100%' }}
              />
            </div>
          ))}
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
