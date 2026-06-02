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

import { useEffect, useState } from 'react';
import { API, showError, showSuccess } from '../../helpers';
import { ITEMS_PER_PAGE } from '../../constants';
import { useTranslation } from 'react-i18next';

export const PROMO_CODE_STATUS = {
  ENABLED: 1,
  DISABLED: 2,
  USED: 3,
};

export const usePromoCodesData = () => {
  const { t } = useTranslation();
  const [promoCodes, setPromoCodes] = useState([]);
  const [plans, setPlans] = useState([]);
  const [loading, setLoading] = useState(false);
  const [searching, setSearching] = useState(false);
  const [activePage, setActivePage] = useState(1);
  const [pageSize, setPageSize] = useState(ITEMS_PER_PAGE);
  const [total, setTotal] = useState(0);
  const [searchKeyword, setSearchKeyword] = useState('');

  const loadPlans = async () => {
    try {
      const res = await API.get('/api/subscription/admin/plans');
      if (res?.data?.success) {
        setPlans((res.data.data || []).map((item) => item.plan || item));
      } else {
        setPlans([]);
      }
    } catch (error) {
      setPlans([]);
    }
  };

  const loadPromoCodes = async (page = activePage, size = pageSize) => {
    setLoading(true);
    try {
      const res = await API.get(`/api/promo-code/?p=${page}&page_size=${size}`);
      if (res?.data?.success) {
        setPromoCodes(res.data.data?.items || []);
        setTotal(res.data.data?.total || 0);
        setActivePage(res.data.data?.page || page);
      } else {
        showError(res?.data?.message || t('优惠码加载失败'));
      }
    } catch (error) {
      showError(error.message || t('优惠码加载失败'));
    } finally {
      setLoading(false);
    }
  };

  const searchPromoCodes = async (
    keyword = searchKeyword,
    page = 1,
    size = pageSize,
  ) => {
    const trimmed = String(keyword || '').trim();
    setSearchKeyword(trimmed);
    if (!trimmed) {
      await loadPromoCodes(page, size);
      return;
    }
    setSearching(true);
    try {
      const res = await API.get(
        `/api/promo-code/search?keyword=${encodeURIComponent(
          trimmed,
        )}&p=${page}&page_size=${size}`,
      );
      if (res?.data?.success) {
        setPromoCodes(res.data.data?.items || []);
        setTotal(res.data.data?.total || 0);
        setActivePage(res.data.data?.page || page);
      } else {
        showError(res?.data?.message || t('优惠码搜索失败'));
      }
    } catch (error) {
      showError(error.message || t('优惠码搜索失败'));
    } finally {
      setSearching(false);
    }
  };

  const refresh = async (page = activePage) => {
    if (searchKeyword) {
      await searchPromoCodes(searchKeyword, page, pageSize);
    } else {
      await loadPromoCodes(page, pageSize);
    }
  };

  const savePromoCode = async (payload) => {
    const res = payload.id
      ? await API.put('/api/promo-code/', payload)
      : await API.post('/api/promo-code/', payload);
    if (res?.data?.success) {
      showSuccess(payload.id ? t('优惠码更新成功') : t('优惠码创建成功'));
      await refresh();
      return true;
    }
    showError(res?.data?.message || t('保存失败'));
    return false;
  };

  const updatePromoCodeStatus = async (record, status) => {
    const res = await API.put('/api/promo-code/?status_only=true', {
      id: record.id,
      status,
    });
    if (res?.data?.success) {
      showSuccess(t('操作成功完成！'));
      await refresh();
    } else {
      showError(res?.data?.message || t('操作失败'));
    }
  };

  const deletePromoCode = async (record) => {
    const res = await API.delete(`/api/promo-code/${record.id}/`);
    if (res?.data?.success) {
      showSuccess(t('删除成功'));
      await refresh();
    } else {
      showError(res?.data?.message || t('删除失败'));
    }
  };

  const handlePageChange = (page) => {
    setActivePage(page);
    if (searchKeyword) {
      searchPromoCodes(searchKeyword, page, pageSize);
    } else {
      loadPromoCodes(page, pageSize);
    }
  };

  const handlePageSizeChange = (size) => {
    setPageSize(size);
    setActivePage(1);
    if (searchKeyword) {
      searchPromoCodes(searchKeyword, 1, size);
    } else {
      loadPromoCodes(1, size);
    }
  };

  useEffect(() => {
    loadPlans();
    loadPromoCodes(1, pageSize);
  }, []);

  return {
    t,
    promoCodes,
    plans,
    loading,
    searching,
    activePage,
    pageSize,
    total,
    searchKeyword,
    setSearchKeyword,
    loadPromoCodes,
    searchPromoCodes,
    refresh,
    savePromoCode,
    updatePromoCodeStatus,
    deletePromoCode,
    handlePageChange,
    handlePageSizeChange,
  };
};
