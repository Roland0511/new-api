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
  useId,
  useState,
  type Dispatch,
  type ReactNode,
  type SetStateAction,
} from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { ModelPricingEndpointVariant } from '@/features/model-pricing/api'
import type { PricingCurrency } from '@/features/model-pricing/currency'
import {
  combineBillingExpr,
  splitBillingExprAndRequestRules,
} from '@/features/pricing/lib/billing-expr'
import {
  endpointLabel,
  isDecisionsEndpoint,
} from '@/features/pricing/lib/endpoint-pricing'

import { SettingsSwitchField } from '../components/settings-form-layout'
import { TieredPricingEditor } from './tiered-pricing-editor'

export function EndpointPricingEditor(props: {
  variants: ModelPricingEndpointVariant[]
  expressions: Record<string, string>
  onChange: Dispatch<SetStateAction<Record<string, string>>>
  modelName: string
  currency: PricingCurrency
  children: ReactNode
}) {
  const { t } = useTranslation()
  const id = useId()
  const [selected, setSelected] = useState('__model__')
  const variants = props.variants.filter((variant) =>
    isDecisionsEndpoint(variant.endpoint_type)
  )
  if (!variants.length) return props.children
  return (
    <Tabs
      value={selected}
      onValueChange={(value) => setSelected(String(value))}
    >
      <TabsList
        aria-label={t('Endpoints')}
        className='max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto'
      >
        <TabsTrigger value='__model__'>{t('Default')}</TabsTrigger>
        {variants.map((variant) => (
          <TabsTrigger
            key={variant.endpoint_type}
            value={variant.endpoint_type}
          >
            {endpointLabel(
              variant.endpoint_type as 'openai-decisions' | 'jev-decisions'
            )}
          </TabsTrigger>
        ))}
      </TabsList>
      <TabsContent value='__model__' keepMounted>
        {props.children}
      </TabsContent>
      {variants.map((variant) => {
        const endpoint = variant.endpoint_type
        const separate = Object.hasOwn(props.expressions, endpoint)
        const fallback =
          variant.builtin ?? (variant.configured ? '' : variant.effective)
        const expression = separate ? props.expressions[endpoint] : fallback
        const split = splitBillingExprAndRequestRules(expression)
        let description: string | undefined
        if (!separate)
          {description = expression
            ? t('Using built-in or default pricing')
            : t('Unset price')}
        return (
          <TabsContent
            key={endpoint}
            value={endpoint}
            keepMounted
            className='space-y-3'
          >
            <p className='text-muted-foreground text-sm'>
              {t(
                'Decisions prices are separate from chat and Responses prices.'
              )}
            </p>
            <SettingsSwitchField
              controlId={`${id}-${endpoint}`}
              checked={separate}
              label={t('Override endpoint pricing')}
              description={description}
              onCheckedChange={(checked) =>
                props.onChange((current) => {
                  const next = { ...current }
                  if (checked) next[endpoint] = fallback
                  else delete next[endpoint]
                  return next
                })
              }
            />
            {separate ? (
              <div
                data-billing-invalid={!expression.trim() ? 'true' : undefined}
              >
                {!expression.trim() && (
                  <Alert variant='destructive'>
                    <AlertDescription>
                      {t('Enter an endpoint price before saving.')}
                    </AlertDescription>
                  </Alert>
                )}
                <TieredPricingEditor
                  key={`${props.modelName}:${endpoint}`}
                  modelName={props.modelName}
                  currency={props.currency}
                  billingExpr={split.billingExpr}
                  requestRuleExpr={split.requestRuleExpr}
                  onBillingExprChange={(next) =>
                    props.onChange((current) => ({
                      ...current,
                      [endpoint]: combineBillingExpr(
                        next,
                        split.requestRuleExpr
                      ),
                    }))
                  }
                  onRequestRuleExprChange={(next) =>
                    props.onChange((current) => ({
                      ...current,
                      [endpoint]: combineBillingExpr(split.billingExpr, next),
                    }))
                  }
                />
              </div>
            ) : null}
            {!separate && expression && (
              <code className='bg-muted/30 block overflow-auto rounded-md border p-3 text-xs break-all whitespace-pre-wrap'>
                {expression}
              </code>
            )}
            {!separate && !expression && (
              <Alert>
                <AlertDescription>
                  {t('Set an endpoint price to enable Decisions requests.')}
                </AlertDescription>
              </Alert>
            )}
          </TabsContent>
        )
      })}
    </Tabs>
  )
}
