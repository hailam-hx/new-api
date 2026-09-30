/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU
Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { afterEach, expect, it } from 'vitest'

import i18n from '@/i18n/config'
import vi from '@/i18n/locales/vi.json'
import zh from '@/i18n/locales/zh.json'
import { getCurrencyLabel } from '@/lib/currency'
import { formatQuota } from '@/lib/format'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

afterEach(async () => {
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  await i18n.changeLanguage('en')
})

it('shows the configured points unit in the selected interface language', async () => {
  i18n.addResourceBundle('vi', 'translation', vi.translation)
  i18n.addResourceBundle('zhCN', 'translation', zh.translation)
  useSystemConfigStore.getState().setConfig({
    currency: {
      ...DEFAULT_CURRENCY_CONFIG,
      quotaDisplayType: 'CUSTOM',
      customCurrencySymbol: '积分',
    },
  })

  await i18n.changeLanguage('vi')
  expect(formatQuota(100_000_000)).toBe('Điểm 200')
  expect(getCurrencyLabel()).toBe('Điểm')

  await i18n.changeLanguage('zhCN')
  expect(formatQuota(100_000_000)).toBe('积分 200')
  expect(getCurrencyLabel()).toBe('积分')
})

it('preserves an arbitrary custom currency symbol', async () => {
  useSystemConfigStore.getState().setConfig({
    currency: {
      ...DEFAULT_CURRENCY_CONFIG,
      quotaDisplayType: 'CUSTOM',
      customCurrencySymbol: '€',
    },
  })

  await i18n.changeLanguage('vi')
  expect(formatQuota(100_000_000)).toBe('€ 200')
  expect(getCurrencyLabel()).toBe('€')
})
