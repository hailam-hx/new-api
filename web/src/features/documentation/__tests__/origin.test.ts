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
import { describe, expect, it } from 'vitest'

import { resolveDocsOrigin } from '../origin'

describe('documentation API origin', () => {
  it('uses the configured gateway prefix without doubling the SDK version path', () => {
    expect(
      resolveDocsOrigin([
        ' https://gateway.example/proxy/v1/ ',
        'https://site.example',
      ])
    ).toBe('https://gateway.example/proxy')
  })
  it('falls back past unsafe, credentialed and relative addresses', () => {
    expect(
      resolveDocsOrigin([
        'javascript:alert(1)',
        'https://user:secret@gateway.example',
        '/relay',
        'https://site.example/',
      ])
    ).toBe('https://site.example')
  })
  it('drops query strings and fragments from a configured native Gemini base', () => {
    expect(
      resolveDocsOrigin(['https://gateway.example/v1beta?x=1#section'])
    ).toBe('https://gateway.example')
  })
})
