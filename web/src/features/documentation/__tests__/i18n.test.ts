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
import { createInstance } from 'i18next'
import { describe, expect, it } from 'vitest'

import en from '@/i18n/locales/en.json'
import fr from '@/i18n/locales/fr.json'
import ja from '@/i18n/locales/ja.json'
import ru from '@/i18n/locales/ru.json'
import vi from '@/i18n/locales/vi.json'
import zhTW from '@/i18n/locales/zh-TW.json'
import zh from '@/i18n/locales/zh.json'

import {
  articles,
  translateDocSection,
  DOC_ERROR_GUIDANCE,
  DEFAULT_ERROR_GUIDANCE,
} from '../content'
import reference from '../generated/reference.json'
import { searchDocumentation } from '../lib'

const locales = { en, fr, ja, ru, vi, zh, 'zh-TW': zhTW }

describe('documentation translations', () => {
  it.each(Object.entries(locales))(
    '%s translates every article paragraph without English fallback',
    (language, locale) => {
      for (const key of [
        ...Object.values(DOC_ERROR_GUIDANCE),
        DEFAULT_ERROR_GUIDANCE,
      ]) {
        const text = locale.translation[key as keyof typeof locale.translation]
        expect(text).toBeTruthy()
        if (language !== 'en') expect(text).not.toBe(key)
      }
      for (const article of articles) {
        for (const key of [
          article.title,
          article.description,
          article.group,
          ...article.sections.map((section) => section.title),
        ]) {
          expect(
            locale.translation[key as keyof typeof locale.translation]
          ).toBeTruthy()
        }
        for (const section of article.sections) {
          const translated =
            locale.translation[section.body as keyof typeof locale.translation]
          expect(
            translated,
            `${language}: ${article.slug}/${section.id}`
          ).toBeTruthy()
          expect(
            [...translated.matchAll(/`([^`]+)`/g)]
              .map((match) => match[1])
              .sort()
          ).toEqual(
            [...section.body.matchAll(/`([^`]+)`/g)]
              .map((match) => match[1])
              .sort()
          )
          expect(
            [...translated.matchAll(/\]\(([^)]+)\)/g)]
              .map((match) => match[1])
              .sort()
          ).toEqual(
            [...section.body.matchAll(/\]\(([^)]+)\)/g)]
              .map((match) => match[1])
              .sort()
          )
          // Provider credential strings can originate in Chinese; technical identifiers stay intact.
          if (
            language !== 'en' &&
            /[a-zA-Z]{3,}.*\s/.test(section.body) &&
            !/[\u3400-\u9fff]/.test(section.body)
          ) {
            expect(
              translated,
              `${language}: ${article.slug}/${section.id}`
            ).not.toBe(section.body)
          }
          expect(
            [...translated.matchAll(/{{([^}]+)}}/g)]
              .map((match) => match[1])
              .sort()
          ).toEqual(
            [...section.body.matchAll(/{{([^}]+)}}/g)]
              .map((match) => match[1])
              .sort()
          )
        }
      }
    }
  )
  it.each(Object.keys(locales))(
    '%s localizes provider details and resolves metadata placeholders',
    async (language) => {
      const instance = createInstance()
      await instance.init({
        lng: language,
        fallbackLng: false,
        nsSeparator: false,
        resources: locales,
      })
      const section = articles.find((article) => article.slug === 'provider-58')
        ?.sections[0]
      if (!section) throw new Error('Missing provider fixture')
      const text = translateDocSection(section, instance.t.bind(instance))
      expect(text).toContain('58')
      expect(text).toContain('New API')
      expect(text).not.toContain('{{')
      if (language !== 'en') expect(text).not.toContain('Channel type')
    }
  )
  it.each(Object.keys(locales))(
    '%s finds documentation by translated paragraph text',
    async (language) => {
      const instance = createInstance()
      await instance.init({
        lng: language,
        fallbackLng: false,
        nsSeparator: false,
        resources: locales,
      })
      const paragraph = articles.find((article) => article.slug === 'overview')
        ?.sections[0].body
      if (!paragraph) throw new Error('Missing overview fixture')
      const results = searchDocumentation(
        instance.t(paragraph),
        articles,
        reference,
        [],
        instance.t.bind(instance)
      )
      expect(results.some((result) => result.href.startsWith('/docs#'))).toBe(
        true
      )
    }
  )
  it('changes article prose immediately when switching the interface language', async () => {
    const instance = createInstance()
    await instance.init({
      lng: 'en',
      fallbackLng: false,
      nsSeparator: false,
      resources: locales,
    })
    const paragraph = articles.find((article) => article.slug === 'overview')
      ?.sections[0].body
    if (!paragraph) throw new Error('Missing overview fixture')
    expect(instance.t(paragraph)).toBe(
      en.translation[paragraph as keyof typeof en.translation]
    )
    for (const language of ['vi', 'zh', 'zh-TW', 'fr', 'ja', 'ru']) {
      await instance.changeLanguage(language)
      expect(instance.t(paragraph)).not.toBe(paragraph)
    }
  })
})
