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
import { act, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'
import { formatTimestampToDate } from '@/lib/format'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import type { UserSubscriptionStatus } from '../../types'
import { UserSubscriptionCell } from '../user-subscription-cell'

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: { en, zhCN: zh },
  initAsync: false,
})
const now = 1792123200
const subscription: UserSubscriptionStatus = {
  id: 1,
  plan_title: 'Monthly',
  amount_used: 2500000,
  amount_total: 10000000,
  end_time: now + 90060,
  next_reset_time: now + 3600,
}

function show(subscriptions?: UserSubscriptionStatus[]) {
  return render(
    <I18nextProvider i18n={i18n}>
      <UserSubscriptionCell subscriptions={subscriptions} />
    </I18nextProvider>
  )
}

beforeEach(async () => {
  vi.spyOn(Date, 'now').mockReturnValue(now * 1000)
  await i18n.changeLanguage('en')
  useSystemConfigStore
    .getState()
    .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
})

afterEach(() => {
  vi.useRealTimers()
})

it('shows consumption and opens expiry, remaining time and reset details by keyboard', async () => {
  show([subscription])
  expect(screen.getByText('$5 / $20')).toBeInTheDocument()
  const user = userEvent.setup()
  await user.tab()
  expect(
    screen.getByRole('button', { name: 'View subscription details: Monthly' })
  ).toHaveFocus()
  await user.keyboard('{Enter}')
  const dialog = screen.getByRole('dialog')
  expect(within(dialog).getByText('1 days 1 hours 1 minutes')).toBeVisible()
  expect(
    within(dialog).getByText(formatTimestampToDate(subscription.end_time))
  ).toBeVisible()
  expect(
    within(dialog).getByText(
      formatTimestampToDate(subscription.next_reset_time)
    )
  ).toBeVisible()
  await user.keyboard('{Escape}')
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  expect(screen.getByRole('button')).toHaveFocus()
})

it('keeps multiple subscriptions separate and supports unlimited quota and no further reset', async () => {
  show([
    subscription,
    {
      ...subscription,
      id: 2,
      plan_title: 'Unlimited plan',
      amount_total: 0,
      next_reset_time: 0,
    },
  ])
  const user = userEvent.setup()
  expect(screen.getAllByRole('button')).toHaveLength(2)
  expect(screen.getByText('$5 / Unlimited')).toBeInTheDocument()
  await user.click(
    screen.getByRole('button', {
      name: 'View subscription details: Unlimited plan',
    })
  )
  expect(screen.getByRole('dialog')).toHaveTextContent(
    'No further resets before expiry'
  )
})

it('distinguishes missing data from no active subscription and hides expired subscriptions', () => {
  const view = show(undefined)
  expect(screen.getByText('Subscription information unavailable')).toBeVisible()
  view.rerender(
    <I18nextProvider i18n={i18n}>
      <UserSubscriptionCell
        subscriptions={[{ ...subscription, end_time: now }]}
      />
    </I18nextProvider>
  )
  expect(screen.getByText('No active subscription')).toBeVisible()
  expect(screen.queryByRole('button')).not.toBeInTheDocument()
})

it('refreshes expiry while the list remains open', () => {
  vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
  show([{ ...subscription, end_time: now + 10 }])
  expect(screen.getByRole('button')).toBeInTheDocument()
  vi.mocked(Date.now).mockReturnValue((now + 30) * 1000)
  act(() => {
    vi.advanceTimersByTime(30_000)
  })
  expect(screen.getByText('No active subscription')).toBeVisible()
})

it('updates Chinese labels and uses the configured currency', async () => {
  await i18n.changeLanguage('zhCN')
  useSystemConfigStore
    .getState()
    .setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: 'CNY',
        usdExchangeRate: 7,
      },
    })
  show([subscription])
  expect(screen.getByText('¥35 / ¥140')).toBeInTheDocument()
  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: '查看订阅详情：Monthly' }))
  expect(screen.getByRole('dialog')).toHaveTextContent('1 天 1 小时 1 分钟')
  expect(screen.getByRole('dialog')).toHaveTextContent('剩余时间')
})
