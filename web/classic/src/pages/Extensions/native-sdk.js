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
  Fragment,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { jsx, jsxs } from 'react/jsx-runtime';
import {
  Banner,
  Button,
  ButtonGroup,
  Card,
  Collapsible,
  DatePicker,
  Empty,
  Input,
  Pagination,
  Progress,
  Select,
  Space,
  Spin,
  Table,
  Tabs,
  Tag,
  Toast,
  Typography,
} from '@douyinfe/semi-ui';
import {
  Activity,
  BadgeDollarSign,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Clock3,
  Database,
  ExternalLink,
  Layers3,
  Play,
  RefreshCw,
  RotateCcw,
  Search,
  Server,
  Sigma,
  SlidersHorizontal,
  TriangleAlert,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { VChart } from '@visactor/react-vchart';
import { initVChartSemiTheme } from '@visactor/vchart-semi-theme';
import { API } from '../../helpers';
import { CHART_CONFIG } from '../../constants/dashboard.constants';

const ReactModule = Object.freeze({
  default: React,
  Fragment,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
});

const ReactJSXRuntime = Object.freeze({ Fragment, jsx, jsxs });

const SemiUI = Object.freeze({
  Banner,
  Button,
  ButtonGroup,
  Card,
  Collapsible,
  DatePicker,
  Empty,
  Input,
  Pagination,
  Progress,
  Select,
  Space,
  Spin,
  Table,
  Tabs,
  Tag,
  Toast,
  Typography,
});

const LucideReact = Object.freeze({
  Activity,
  BadgeDollarSign,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Clock3,
  Database,
  ExternalLink,
  Layers3,
  Play,
  RefreshCw,
  RotateCcw,
  Search,
  Server,
  Sigma,
  SlidersHorizontal,
  TriangleAlert,
});

const ReactI18next = Object.freeze({ useTranslation });
const ReactVChart = Object.freeze({ VChart });
const VChartSemiTheme = Object.freeze({ initVChartSemiTheme });
const Helpers = Object.freeze({ API });
const DashboardConstants = Object.freeze({ CHART_CONFIG });

const modules = Object.freeze({
  react: ReactModule,
  'react/jsx-runtime': ReactJSXRuntime,
  '@douyinfe/semi-ui': SemiUI,
  'lucide-react': LucideReact,
  'react-i18next': ReactI18next,
  '@visactor/react-vchart': ReactVChart,
  '@visactor/vchart-semi-theme': VChartSemiTheme,
  '../../helpers': Helpers,
  '../../constants/dashboard.constants': DashboardConstants,
});

export const classicNativeExtensionSdk = Object.freeze({
  platform: 'classic',
  sdk: 'v1',
  modules,
});

Object.defineProperty(globalThis, '__NEW_API_EXTENSION_NATIVE_SDK__', {
  configurable: true,
  enumerable: false,
  writable: false,
  value: classicNativeExtensionSdk,
});
