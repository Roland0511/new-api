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
import { describe, expect, test } from 'vitest'

import {
  CHANNEL_TYPE_OPTIONS,
  CHANNEL_TYPE_TASK_PLUGIN,
  channelTypeOptionsForTaskPluginBind,
} from '../../constants'
import {
  CHANNEL_TYPE_CONFIGS,
  getRelatedChannelModels,
} from '../channel-type-config'

test('channel choices exclude standalone TypeSafe while preserving OpenAI model filtering', () => {
  expect(CHANNEL_TYPE_OPTIONS.some((option) => option.label === 'TypeSafe')).toBe(false)
  expect(CHANNEL_TYPE_CONFIGS[64]).toBeUndefined()
  expect(
    getRelatedChannelModels(1, [
      'gpt-6-luna',
      'text-embedding-3-small',
      'jev-latest',
    ])
  ).toEqual(['gpt-6-luna', 'text-embedding-3-small'])
})

describe('channel type options for task plugin bind', () => {
  test('hides the task plugin type when the caller cannot bind', () => {
    const options = channelTypeOptionsForTaskPluginBind(false)

    expect(
      options.some((option) => option.value === CHANNEL_TYPE_TASK_PLUGIN)
    ).toBe(false)
  })

  test('shows the task plugin type when the caller can bind', () => {
    const options = channelTypeOptionsForTaskPluginBind(true)

    expect(options).toEqual(CHANNEL_TYPE_OPTIONS)
    expect(
      options.some((option) => option.value === CHANNEL_TYPE_TASK_PLUGIN)
    ).toBe(true)
  })
})
