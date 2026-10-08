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
import type { PricingModel } from '../types'

export const DECISIONS_ENDPOINTS = [
  'openai-decisions',
  'jev-decisions',
] as const
export type DecisionsEndpoint = (typeof DECISIONS_ENDPOINTS)[number]

export function isDecisionsEndpoint(value: string): value is DecisionsEndpoint {
  return DECISIONS_ENDPOINTS.some((endpoint) => endpoint === value)
}

export function splitEndpointBillingExprKey(
  key: string
): [DecisionsEndpoint, string] | null {
  const separator = key.indexOf('::')
  const endpoint = key.slice(0, separator)
  const model = key.slice(separator + 2)
  return separator > 0 && isDecisionsEndpoint(endpoint) && model.trim()
    ? [endpoint, model]
    : null
}

export function endpointLabel(endpoint: DecisionsEndpoint) {
  return endpoint === 'openai-decisions' ? 'OpenAI Decisions' : 'JEV Decisions'
}

export function endpointPricingModel(
  model: PricingModel,
  expression: string
): PricingModel {
  return {
    ...model,
    billing_endpoint_variants: undefined,
    billing_plugin_variants: undefined,
    billing_mode: expression ? 'tiered_expr' : undefined,
    billing_expr: expression || undefined,
    quota_type: 0,
    model_ratio: Number.NaN,
    completion_ratio: Number.NaN,
    model_price: undefined,
    cache_ratio: undefined,
    create_cache_ratio: undefined,
    audio_ratio: undefined,
    audio_completion_ratio: undefined,
    image_ratio: undefined,
  }
}
