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
import type { TFunction } from 'i18next'

export type ChannelTestResult = {
  status: 'idle' | 'testing' | 'success' | 'error'
  responseTime?: number
  completedAt?: number
  error?: string
  errorCode?: string
  endpointType?: string
  stream?: boolean
}

export type ChannelTestExportProps = {
  channelId: number
  channelName: string
  models: string[]
  results: Record<string, ChannelTestResult>
  disabled?: boolean
}

export function createChannelTestCSV(
  props: ChannelTestExportProps,
  t: TFunction
): string {
  const statuses = {
    idle: t('Not tested'),
    testing: t('Testing'),
    success: t('Success'),
    error: t('Failed'),
  }
  const rows: (string | number | boolean)[][] = [
    [
      t('Channel ID'),
      t('Name'),
      t('Model'),
      t('Status'),
      t('Response time (s)'),
      t('Completed at (UTC)'),
      t('Endpoint Type'),
      t('Stream Mode'),
      t('Reason code'),
      t('Details'),
      t('Test cost'),
    ],
  ]
  for (const model of props.models) {
    const result = props.results[model]
    rows.push([
      props.channelId,
      props.channelName,
      model,
      statuses[result?.status ?? 'idle'],
      result?.responseTime ?? '',
      result?.completedAt ? new Date(result.completedAt).toISOString() : '',
      result?.endpointType ?? '',
      result?.stream ?? '',
      result?.errorCode ?? '',
      result?.error ?? '',
      t('Unknown'),
    ])
  }
  const content = rows
    .map((row) =>
      row
        .map((value) => {
          let text = String(value)
          // Upstream/model text must remain literal when opened in spreadsheet apps.
          if (/^[\s]*[=+\-@]/.test(text) || /^[\t\r\n]/.test(text)) {
            text = `'${text}`
          }
          return `"${text.replaceAll('"', '""')}"`
        })
        .join(',')
    )
    .join('\r\n')
  return `\uFEFF${content}\r\n`
}
