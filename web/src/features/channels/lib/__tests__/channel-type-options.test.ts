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
  CHANNEL_TYPE_TYPESAFE,
  channelTypeOptionsForTaskPluginBind,
} from '../../constants'
import { CHANNEL_TYPE_CONFIGS } from '../channel-type-config'
import { getChannelTypeIcon } from '../channel-utils'

test('TypeSafe keeps its native model presets and vendor identity', () => {
  expect(getChannelTypeIcon(CHANNEL_TYPE_TYPESAFE)).toBe('TypeSafe')
  expect(CHANNEL_TYPE_CONFIGS[CHANNEL_TYPE_TYPESAFE].icon).toBe('TypeSafe')
  expect(CHANNEL_TYPE_CONFIGS[CHANNEL_TYPE_TYPESAFE].supportedModels).toEqual([
    'jev-latest',
    'jev-preview',
    'jev-1.13.0',
  ])
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
