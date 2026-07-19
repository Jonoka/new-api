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

export const GROUP_DETAILS_UPDATED_EVENT = 'new-api:group-details-updated';

export const notifyGroupDetailsUpdated = () => {
  if (typeof window === 'undefined') return;
  window.dispatchEvent(new window.Event(GROUP_DETAILS_UPDATED_EVENT));
};

const normalizeId = (value) => {
  const id = Number(value);
  return Number.isInteger(id) && id > 0 ? id : null;
};

const normalizeRatio = (value) => {
  const ratio = Number(value);
  return Number.isFinite(ratio) && ratio >= 0 ? ratio : 1;
};

const normalizeStatus = (value) => {
  if (value === 0 || value === '0' || value === 'disabled') return 0;
  return 1;
};

export const createUniqueGroupCode = (groupsOrCodes = []) => {
  const existingCodes = new Set();
  for (const item of groupsOrCodes || []) {
    const value = typeof item === 'object' && item !== null ? item.code : item;
    const code = String(value ?? '').trim();
    if (code) existingCodes.add(code);
  }

  let index = 1;
  while (existingCodes.has(`group_${index}`)) index += 1;
  return `group_${index}`;
};

export const getGroupDisplayName = (code, groupNames = {}) => {
  const normalizedCode = String(code ?? '').trim();
  if (!normalizedCode) return '';

  const mappedName =
    groupNames && typeof groupNames === 'object'
      ? groupNames[normalizedCode]
      : '';
  return String(mappedName ?? '').trim() || normalizedCode;
};

export const normalizeGroupDetail = (group = {}) => {
  const code = String(group.code ?? '').trim();
  const name = String(group.name ?? '').trim();
  const autoOrder = Number(group.auto_order);

  return {
    id: normalizeId(group.id),
    code,
    name: name || code,
    description: String(group.description ?? ''),
    ratio: normalizeRatio(group.ratio),
    user_selectable: group.user_selectable === true,
    status: normalizeStatus(group.status),
    auto_enabled: group.auto_enabled === true,
    auto_order:
      group.auto_enabled === true &&
      Number.isInteger(autoOrder) &&
      autoOrder >= 0
        ? autoOrder
        : 0,
  };
};

export const normalizeGroupDetails = (groups) =>
  Array.isArray(groups) ? groups.map(normalizeGroupDetail) : [];

export const extractGroupDetailsResponse = (payload) => {
  if (Array.isArray(payload)) return normalizeGroupDetails(payload);
  if (Array.isArray(payload?.groups)) {
    return normalizeGroupDetails(payload.groups);
  }
  if (Array.isArray(payload?.data)) {
    return normalizeGroupDetails(payload.data);
  }
  if (Array.isArray(payload?.data?.groups)) {
    return normalizeGroupDetails(payload.data.groups);
  }
  return null;
};

export const formatGroupLabel = (group = {}) => {
  const code = String(group.code ?? group.value ?? '').trim();
  const name = String(group.name ?? '').trim();
  return name && name !== code ? `${name} (${code})` : name || code;
};

export const createGroupOptions = (groups) =>
  normalizeGroupDetails(groups).map((group) => ({
    ...group,
    value: group.code,
    label: group.name || group.code,
  }));

export const createUserGroupOptions = (groupMap) =>
  Object.entries(groupMap || {}).map(([mapCode, info = {}]) => {
    const code = String(info.code || mapCode).trim();
    const isAuto = code === 'auto';
    const name = String(info.name || code).trim();
    return {
      id: isAuto ? null : normalizeId(info.id),
      code,
      name,
      legacy_code: String(mapCode).trim(),
      value: code,
      label: name || code,
      description: String(info.description || info.desc || ''),
      ratio: info.ratio,
    };
  });

export const groupDetailsToLegacyOptions = (groups) => {
  const normalized = normalizeGroupDetails(groups);
  const groupRatio = {};
  const userUsableGroups = {};
  const autoGroups = normalized
    .filter((group) => group.auto_enabled && group.code)
    .sort((a, b) => a.auto_order - b.auto_order)
    .map((group) => group.code);

  normalized.forEach((group) => {
    if (!group.code) return;
    groupRatio[group.code] = group.ratio;
    if (group.user_selectable) {
      userUsableGroups[group.code] = group.description;
    }
  });

  return {
    GroupRatio: JSON.stringify(groupRatio, null, 2),
    UserUsableGroups: JSON.stringify(userUsableGroups, null, 2),
    AutoGroups: autoGroups.length > 0 ? JSON.stringify(autoGroups) : '',
  };
};

export const applyAutoGroupCodes = (groups, codes) => {
  const orderByCode = new Map(
    (Array.isArray(codes) ? codes : []).map((code, index) => [code, index]),
  );

  return normalizeGroupDetails(groups).map((group) => ({
    ...group,
    auto_enabled: orderByCode.has(group.code),
    auto_order: orderByCode.get(group.code) ?? 0,
  }));
};

export const reorderAutoGroupItems = (
  items,
  sourceId,
  targetId,
  position = 'before',
) => {
  const currentItems = Array.isArray(items) ? items : [];
  if (!sourceId || !targetId || sourceId === targetId) return currentItems;

  const sourceIndex = currentItems.findIndex((item) => item?._id === sourceId);
  const targetIndex = currentItems.findIndex((item) => item?._id === targetId);
  if (sourceIndex < 0 || targetIndex < 0) return currentItems;

  const reorderedItems = [...currentItems];
  const [movedItem] = reorderedItems.splice(sourceIndex, 1);
  const remainingTargetIndex = reorderedItems.findIndex(
    (item) => item?._id === targetId,
  );
  const insertIndex = remainingTargetIndex + (position === 'after' ? 1 : 0);
  reorderedItems.splice(insertIndex, 0, movedItem);

  return reorderedItems.every((item, index) => item === currentItems[index])
    ? currentItems
    : reorderedItems;
};

export const parseAutoGroupCodes = (value) => {
  if (!value || !String(value).trim()) return [];
  try {
    const parsed = JSON.parse(value);
    return Array.isArray(parsed)
      ? parsed.map((code) => String(code).trim()).filter(Boolean)
      : [];
  } catch {
    return [];
  }
};

export const getDeletedGroupIds = (originalGroups, currentGroups) => {
  const currentIds = new Set(
    normalizeGroupDetails(currentGroups)
      .map((group) => group.id)
      .filter(Boolean),
  );
  return normalizeGroupDetails(originalGroups)
    .map((group) => group.id)
    .filter((id) => id && !currentIds.has(id));
};

export const buildGroupDetailsPayload = (groups, deletedIds = []) => ({
  groups: normalizeGroupDetails(groups).map((group) => ({
    id: group.id || 0,
    code: group.code,
    name: group.name,
    description: group.description,
    ratio: group.ratio,
    user_selectable: group.user_selectable,
    status: group.status,
    auto_enabled: group.auto_enabled,
    auto_order: group.auto_order,
  })),
  deleted_ids: Array.from(
    new Set((deletedIds || []).map(normalizeId).filter(Boolean)),
  ),
});

export const buildGroupSelectionPayload = (selectedCodes, groupOptions) => {
  const codes = (Array.isArray(selectedCodes) ? selectedCodes : [])
    .map((code) => String(code).trim())
    .filter(Boolean);

  if (codes.length === 1 && codes[0] === 'auto') {
    return { group: 'auto', group_ids: [], group_mode: 'auto' };
  }

  if (codes.length === 0) {
    return { group: '', group_ids: [], group_mode: 'inherit' };
  }

  const idByCode = new Map(
    (groupOptions || [])
      .filter((group) => group?.code && normalizeId(group.id))
      .map((group) => [String(group.code), normalizeId(group.id)]),
  );
  const ids = codes.map((code) => idByCode.get(code));

  return {
    group: codes.join(','),
    group_ids: ids.every(Boolean) ? ids : undefined,
    group_mode: 'explicit',
  };
};

export const resolveGroupCodes = (record, groupOptions) => {
  const mode = String(record?.group_mode || '').trim();
  if (mode === 'auto') return ['auto'];
  if (mode === 'inherit') return [];

  const options = Array.isArray(groupOptions) ? groupOptions : [];
  const references = Array.isArray(record?.group_details)
    ? record.group_details
    : [];
  const groupIds = (Array.isArray(record?.group_ids) ? record.group_ids : [])
    .map(normalizeId)
    .filter(Boolean);
  if (groupIds.length > 0) {
    const codeById = new Map();
    references.forEach((group) => {
      const id = normalizeId(group?.id);
      const code = String(group?.code || '').trim();
      if (id && code) codeById.set(id, code);
    });
    options.forEach((group) => {
      const id = normalizeId(group?.id);
      const code = String(group?.code || '').trim();
      if (id && code) codeById.set(id, code);
    });
    const resolvedCodes = groupIds.map((id) => codeById.get(id));
    if (resolvedCodes.every(Boolean)) {
      return resolvedCodes;
    }
  }

  const referenceCodes = references
    .map((group) => String(group?.code || '').trim())
    .filter(Boolean);
  if (referenceCodes.length > 0) return referenceCodes;

  const canonicalCodeByLegacyCode = new Map();
  options.forEach((group) => {
    const code = String(group?.code || group?.value || '').trim();
    if (!code) return;
    canonicalCodeByLegacyCode.set(code, code);
    const legacyCode = String(group?.legacy_code || '').trim();
    if (legacyCode) canonicalCodeByLegacyCode.set(legacyCode, code);
  });
  const legacyCodes = String(record?.group || '')
    .split(',')
    .map((code) => code.trim())
    .filter(Boolean);
  return legacyCodes.map((code) => canonicalCodeByLegacyCode.get(code) || code);
};
