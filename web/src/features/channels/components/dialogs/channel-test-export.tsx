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
import { Download } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { handleServerError } from '@/lib/handle-server-error'

import {
  createChannelTestCSV,
  type ChannelTestExportProps,
} from '../../lib/channel-test-export'

export function ChannelTestExport(props: ChannelTestExportProps) {
  const { t } = useTranslation()
  const handleExport = () => {
    try {
      const blob = new Blob([createChannelTestCSV(props, t)], {
        type: 'text/csv;charset=utf-8',
      })
      const url = URL.createObjectURL(blob)
      try {
        const link = document.createElement('a')
        link.href = url
        link.download = `channel-${props.channelId}-tests-${Date.now()}.csv`
        document.body.append(link)
        link.click()
        link.remove()
      } finally {
        URL.revokeObjectURL(url)
      }
    } catch (error) {
      handleServerError(error)
    }
  }
  return (
    <div className='space-y-1'>
      <Button
        variant='outline'
        size='sm'
        onClick={handleExport}
        disabled={
          props.disabled ||
          (props.models.length === 0 && !props.verification?.items.length)
        }
      >
        <Download data-icon='inline-start' />
        {t('Export CSV')}
      </Button>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Export all models, verification layers, and sanitized evidence for Excel. Unknown costs remain blank.'
        )}
      </p>
    </div>
  )
}
