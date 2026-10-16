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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { QuotaDetailsPopover } from '@/components/quota-details-popover'
import { StatusBadge } from '@/components/status-badge'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'
import { useSystemConfigStore } from '@/stores/system-config-store'

import type { UserSubscriptionStatus } from '../types'

export function UserSubscriptionCell(props: {
  subscriptions?: UserSubscriptionStatus[]
}) {
  const { t } = useTranslation()
  useSystemConfigStore((state) => state.config.currency)
  const [now, setNow] = useState(() => Math.floor(Date.now() / 1000))

  useEffect(() => {
    const timer = window.setInterval(
      () => setNow(Math.floor(Date.now() / 1000)),
      30_000
    )
    return () => window.clearInterval(timer)
  }, [])

  if (!props.subscriptions) {
    return (
      <span className='text-muted-foreground text-xs'>
        {t('Subscription information unavailable')}
      </span>
    )
  }

  const subscriptions = props.subscriptions.filter((sub) => sub.end_time > now)
  if (subscriptions.length === 0) {
    return (
      <StatusBadge
        label={t('No active subscription')}
        variant='neutral'
        copyable={false}
        className='-ml-1.5 font-normal'
      />
    )
  }

  return (
    <div className='flex min-w-0 flex-col gap-2'>
      {subscriptions.map((sub) => {
        const title = sub.plan_title || `${t('Subscription')} #${sub.id}`
        const used = formatQuotaWithCurrency(sub.amount_used)
        const total =
          sub.amount_total > 0
            ? formatQuotaWithCurrency(sub.amount_total)
            : t('Unlimited')
        const remainingMinutes = Math.floor((sub.end_time - now) / 60)
        const remainingTime =
          remainingMinutes < 1
            ? t('Less than a minute')
            : t('{{days}} days {{hours}} hours {{minutes}} minutes', {
                days: Math.floor(remainingMinutes / 1440),
                hours: Math.floor((remainingMinutes % 1440) / 60),
                minutes: remainingMinutes % 60,
              })
        const nextReset =
          sub.next_reset_time > 0 && sub.next_reset_time < sub.end_time
            ? formatTimestampToDate(sub.next_reset_time)
            : t('No further resets before expiry')

        return (
          <QuotaDetailsPopover
            key={sub.id}
            title={title}
            triggerLabel={t('View subscription details: {{name}}', {
              name: title,
            })}
            details={[
              { label: t('Used amount'), value: used },
              { label: t('Total Quota'), value: total },
              { label: t('Time remaining'), value: remainingTime },
              {
                label: t('Expires at'),
                value: formatTimestampToDate(sub.end_time),
              },
              { label: t('Next reset'), value: nextReset },
            ]}
          >
            <span className='grid min-w-0 gap-y-1 text-sm tabular-nums'>
              <span
                className={cn(
                  sub.amount_total > 0 &&
                    sub.amount_used >= sub.amount_total &&
                    'text-destructive'
                )}
              >
                <span className='text-muted-foreground text-xs'>
                  {t('Used amount')}{' '}
                </span>
                {used} / {total}
              </span>
              <span
                data-table-text='secondary'
                className='text-muted-foreground truncate text-xs'
              >
                {title}
              </span>
            </span>
          </QuotaDetailsPopover>
        )
      })}
    </div>
  )
}
