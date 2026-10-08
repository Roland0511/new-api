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
import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, expect, it } from 'vitest'

import {
  buildPricingChanges,
  type ModelPricingConfig,
} from '@/features/model-pricing/api'
import { USD_PRICING_CURRENCY } from '@/features/model-pricing/currency'
import {
  applyPricingDraft,
  pricingFromDraft,
  pricingOptions,
  pricingRow,
} from '@/features/model-pricing/pricing'
import { EndpointPricingEditor } from '@/features/system-settings/models/endpoint-pricing-editor'

import { ModelPriceCell } from '../components/model-price-cell'
import { buildDecisionsSample } from '../lib/decisions-sample'
import { endpointPricingModel } from '../lib/endpoint-pricing'
import type { PricingModel } from '../types'

afterEach(cleanup)

const model: PricingModel = {
  id: 1,
  model_name: 'shared',
  quota_type: 0,
  model_ratio: 99,
  completion_ratio: 9,
  enable_groups: ['default'],
  supported_endpoint_types: ['openai-response', 'openai-decisions'],
  billing_mode: 'tiered_expr',
  billing_expr: 'p * 99 + c * 88',
  billing_endpoint_variants: [
    { endpoint_type: 'openai-decisions', effective: 'p * 0.1 + c * 0' },
  ],
}

it('renders endpoint prices independently and never inherits the base price for an unset endpoint', () => {
  render(<ModelPriceCell model={model} />)
  expect(screen.getByText('OpenAI Decisions')).toBeInTheDocument()
  expect(screen.getByText('Default')).toBeInTheDocument()
  const endpoint = screen.getByText('OpenAI Decisions').parentElement
  expect(endpoint?.textContent).toContain('0.1')
  expect(endpoint?.textContent).not.toContain('99')
  cleanup()
  render(
    <ModelPriceCell
      model={{
        ...model,
        supported_endpoint_types: ['jev-decisions'],
        billing_endpoint_variants: [
          { endpoint_type: 'jev-decisions', effective: '' },
        ],
      }}
    />
  )
  expect(screen.getByText('Unset price')).toBeInTheDocument()
  expect(screen.queryByText('Default')).not.toBeInTheDocument()
  const unset = endpointPricingModel(model, '')
  expect(Number.isNaN(unset.model_ratio)).toBe(true)
  expect(unset.billing_expr).toBeUndefined()
})

it('roundtrips endpoint overrides, does not copy them to other models, and explicitly clears the last override', () => {
  const options = pricingOptions({
    ModelRatio: '{"shared":99,"target":3}',
    EndpointBillingExpr:
      '{"openai-decisions::shared":"p * 0.2","jev-decisions::target":"p * 0.3"}',
  })
  const configured = {
    ModelRatio: 99,
    'billing_setting.endpoint_billing_expr': { 'openai-decisions': 'p * 0.2' },
  }
  expect(
    pricingFromDraft(pricingRow('shared', configured))[
      'billing_setting.endpoint_billing_expr'
    ]
  ).toEqual({ 'openai-decisions': 'p * 0.2' })
  const copied = applyPricingDraft(
    options,
    {
      name: 'shared',
      billingMode: 'per-token',
      ratio: '1',
      endpointBillingExpr: { 'openai-decisions': 'p * 0.4' },
    },
    ['shared', 'target']
  )
  expect(JSON.parse(copied['billing_setting.endpoint_billing_expr'] ?? '{}')).toEqual({
    'openai-decisions::shared': 'p * 0.4',
    'jev-decisions::target': 'p * 0.3',
  })
  const snapshot: ModelPricingConfig = {
    options,
    empty_version: 'empty',
    entries: [
      {
        model_name: 'shared',
        version: 'v1',
        configured,
        effective: configured,
      },
    ],
  }
  const cleared = {
    ...options,
    'billing_setting.endpoint_billing_expr':
      '{"jev-decisions::target":"p * 0.3"}',
  }
  expect(buildPricingChanges(snapshot, options, cleared)).toEqual([
    {
      model_name: 'shared',
      expected_version: 'v1',
      pricing: { ModelRatio: 99, 'billing_setting.endpoint_billing_expr': {} },
    },
  ])
  expect(buildPricingChanges(snapshot, options, { ...options })).toEqual([])
})

function EndpointEditorFixture() {
  const [expressions, onChange] = useState<Record<string, string>>({})
  return (
    <EndpointPricingEditor
      modelName='unknown'
      currency={USD_PRICING_CURRENCY}
      expressions={expressions}
      onChange={onChange}
      variants={[
        { endpoint_type: 'jev-decisions', configured: '', effective: '' },
      ]}
    >
      <p>Default price 99</p>
    </EndpointPricingEditor>
  )
}

it('requires an explicit endpoint price and does not turn an unknown price into zero automatically', async () => {
  const user = userEvent.setup()
  render(<EndpointEditorFixture />)
  await user.click(screen.getByRole('tab', { name: 'JEV Decisions' }))
  const panel = screen.getByRole('tabpanel')
  expect(within(panel).getByText('Unset price')).toBeInTheDocument()
  await user.click(
    screen.getByRole('switch', { name: 'Override endpoint pricing' })
  )
  expect(
    within(panel).getByText('Enter an endpoint price before saving.')
  ).toBeInTheDocument()
  expect(panel.querySelector('[data-billing-invalid="true"]')).not.toBeNull()
})

it.each(['openai-decisions', 'jev-decisions'])(
  'generates native non-streaming examples for %s',
  (endpointType) => {
    for (const lang of [
      'curl',
      'python',
      'typescript',
      'javascript',
    ] as const) {
      const sample = buildDecisionsSample(lang, {
        baseUrl: 'https://example.test',
        apiKeyEnv: 'GATEWAY_API_KEY',
        modelName: 'shared',
        endpointType,
        endpointPath: '/v1/decisions',
      })
      expect(sample).toContain('https://example.test/v1/decisions')
      expect(sample).toContain('GATEWAY_API_KEY')
      expect(sample).not.toContain('chat/completions')
      expect(sample).not.toContain('stream')
      expect(sample).toContain(
        endpointType === 'openai-decisions' ? 'predicate' : 'noul'
      )
      expect(sample).toContain(
        endpointType === 'openai-decisions' ? 'input' : 'state'
      )
      expect(sample).not.toMatch(/\n\+/)
    }
  }
)
