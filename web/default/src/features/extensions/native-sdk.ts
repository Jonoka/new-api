/*
Copyright (C) 2023-2026 QuantumNous

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
import {
  Fragment as ReactFragment,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import { Fragment as JsxFragment, jsx, jsxs } from 'react/jsx-runtime'
import { useQuery } from '@tanstack/react-query'
import {
  Alert02Icon,
  Analytics01Icon,
  ArrowDown01Icon,
  ArrowRight01Icon,
  ChartRelationshipIcon,
  Database01Icon,
  FilterHorizontalIcon,
  FilterResetIcon,
  InformationCircleIcon,
  Loading03Icon,
  RefreshIcon,
  Router01Icon,
  Search01Icon,
  TestTube01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { CartesianGrid, Line, LineChart, XAxis, YAxis } from 'recharts'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
} from '@/components/ui/chart'
import { Collapsible, CollapsibleContent } from '@/components/ui/collapsible'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group'
import {
  Progress,
  ProgressLabel,
  ProgressValue,
} from '@/components/ui/progress'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { SectionPageLayout } from '@/components/layout'

export const NATIVE_EXTENSION_SDK_VERSION = 'v1' as const
export const NATIVE_EXTENSION_PLATFORM = 'default' as const

export type NativeExtensionSdk = {
  readonly platform: typeof NATIVE_EXTENSION_PLATFORM
  readonly sdk: typeof NATIVE_EXTENSION_SDK_VERSION
  readonly modules: Readonly<Record<string, unknown>>
}

declare global {
  var __NEW_API_EXTENSION_NATIVE_SDK__: NativeExtensionSdk | undefined
}

const modules: NativeExtensionSdk['modules'] = Object.freeze({
  react: Object.freeze({
    Fragment: ReactFragment,
    useEffect,
    useMemo,
    useRef,
    useState,
  }),
  'react/jsx-runtime': Object.freeze({ Fragment: JsxFragment, jsx, jsxs }),
  '@tanstack/react-query': Object.freeze({ useQuery }),
  '@hugeicons/core-free-icons': Object.freeze({
    Alert02Icon,
    Analytics01Icon,
    ArrowDown01Icon,
    ArrowRight01Icon,
    ChartRelationshipIcon,
    Database01Icon,
    FilterHorizontalIcon,
    FilterResetIcon,
    InformationCircleIcon,
    Loading03Icon,
    RefreshIcon,
    Router01Icon,
    Search01Icon,
    TestTube01Icon,
  }),
  '@hugeicons/react': Object.freeze({ HugeiconsIcon }),
  'react-i18next': Object.freeze({ useTranslation }),
  recharts: Object.freeze({ CartesianGrid, Line, LineChart, XAxis, YAxis }),
  sonner: Object.freeze({ toast }),
  '@/lib/api': Object.freeze({ api }),
  '@/lib/utils': Object.freeze({ cn }),
  '@/components/layout': Object.freeze({ SectionPageLayout }),
  '@/components/ui/alert': Object.freeze({
    Alert,
    AlertDescription,
    AlertTitle,
  }),
  '@/components/ui/badge': Object.freeze({ Badge }),
  '@/components/ui/button': Object.freeze({ Button }),
  '@/components/ui/card': Object.freeze({
    Card,
    CardContent,
    CardDescription,
    CardHeader,
    CardTitle,
  }),
  '@/components/ui/chart': Object.freeze({
    ChartContainer,
    ChartLegend,
    ChartLegendContent,
    ChartTooltip,
    ChartTooltipContent,
  }),
  '@/components/ui/collapsible': Object.freeze({
    Collapsible,
    CollapsibleContent,
  }),
  '@/components/ui/empty': Object.freeze({
    Empty,
    EmptyContent,
    EmptyDescription,
    EmptyHeader,
    EmptyMedia,
    EmptyTitle,
  }),
  '@/components/ui/field': Object.freeze({ Field, FieldGroup, FieldLabel }),
  '@/components/ui/input': Object.freeze({ Input }),
  '@/components/ui/input-group': Object.freeze({
    InputGroup,
    InputGroupAddon,
    InputGroupInput,
  }),
  '@/components/ui/progress': Object.freeze({
    Progress,
    ProgressLabel,
    ProgressValue,
  }),
  '@/components/ui/select': Object.freeze({
    Select,
    SelectContent,
    SelectGroup,
    SelectItem,
    SelectTrigger,
    SelectValue,
  }),
  '@/components/ui/skeleton': Object.freeze({ Skeleton }),
  '@/components/ui/table': Object.freeze({
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
  }),
  '@/components/ui/tabs': Object.freeze({
    Tabs,
    TabsContent,
    TabsList,
    TabsTrigger,
  }),
  '@/components/ui/toggle-group': Object.freeze({
    ToggleGroup,
    ToggleGroupItem,
  }),
})

const defaultNativeExtensionSdk: NativeExtensionSdk = Object.freeze({
  platform: NATIVE_EXTENSION_PLATFORM,
  sdk: NATIVE_EXTENSION_SDK_VERSION,
  modules,
})

export function registerDefaultNativeExtensionSdk(): NativeExtensionSdk {
  globalThis.__NEW_API_EXTENSION_NATIVE_SDK__ = defaultNativeExtensionSdk
  return defaultNativeExtensionSdk
}
