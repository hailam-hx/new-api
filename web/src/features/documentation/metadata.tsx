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
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

export function DocumentationMetadata(props: {
  title: string
  description: string
  canonical: string
}) {
  const { t } = useTranslation()
  const title = `${props.title} · ${t('New API Documentation')}`
  useEffect(() => {
    // React hoists its own tags, but does not adopt head tags from the SPA template.
    // Temporarily remove those defaults and restore them when leaving documentation.
    const defaults = [
      ...document.head.querySelectorAll(
        'title, meta[name="title"], meta[name="description"], meta[property^="og:"], link[rel="canonical"]'
      ),
    ].filter((tag) => !tag.hasAttribute('data-new-api-docs-meta'))
    for (const tag of defaults) tag.remove()
    return () => {
      for (const tag of defaults) document.head.append(tag)
    }
  }, [])
  return (
    <>
      <title data-new-api-docs-meta=''>{title}</title>
      <meta
        data-new-api-docs-meta=''
        name='description'
        content={props.description}
      />
      <meta data-new-api-docs-meta='' property='og:title' content={title} />
      <meta
        data-new-api-docs-meta=''
        property='og:description'
        content={props.description}
      />
      <meta data-new-api-docs-meta='' property='og:type' content='article' />
      <meta
        data-new-api-docs-meta=''
        property='og:url'
        content={props.canonical}
      />
      <link data-new-api-docs-meta='' rel='canonical' href={props.canonical} />
    </>
  )
}
