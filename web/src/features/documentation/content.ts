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
import data from './beginner-data.json'

export type DocSection = {
  id: string
  title: string
  body: string
  task?: string
  values?: Record<string, string | number>
}
export type DocArticle = {
  slug: string
  group: string
  title: string
  navigationTitle?: string
  description: string
  sources: string[]
  sections: DocSection[]
  kind: string
  console?: string
  protocol?: string
  keywords?: string[]
}

export const groups: string[] = data.groups
export const articles: DocArticle[] = data.articles
export const beginnerArticles: DocArticle[] = articles.filter(
  (article) => article.slug !== 'hotx-media'
)
export const getArticle = (slug: string): DocArticle | undefined =>
  articles.find((article) => article.slug === slug)

export function translateDocSection(
  section: DocSection,
  translate: (key: string, values?: Record<string, string | number>) => string
): string {
  const values = Object.fromEntries(
    Object.entries(section.values ?? {}).map(([key, value]) => [
      key,
      typeof value === 'string' ? translate(value) : value,
    ])
  )
  return translate(section.body, values)
}
