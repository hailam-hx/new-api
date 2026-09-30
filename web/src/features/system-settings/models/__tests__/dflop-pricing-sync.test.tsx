import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { DflopPricingSync } from '../dflop-pricing-sync'

afterEach(() => vi.restoreAllMocks())

it('keeps apply disabled when a DFLOP preview contains only unsupported pricing', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data: {
      success: true,
      data: url.endsWith('/config')
        ? {
            enabled: true, auto_sync_enabled: false, auto_apply_enabled: false,
            sync_interval_hours: 6, cny_to_usd: '0.15', markup_multiplier: '1',
            sync_text: true, sync_image: true, sync_video: true, sync_audio: true, sync_other: true,
            include_new_callable_models: true, max_auto_increase_percent: '20',
            max_auto_decrease_percent: '50', allow_auto_apply_new_models: false,
          }
        : [],
    },
  }))
  vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: {
        run: { id: 'preview', status: 'preview', source_hash: 'hash', pricing_version_before: 'version', started_at: Math.floor(Date.now() / 1000), changed_count: 0, blocked_count: 1 },
        items: [{ model_id: 'video-per-second', category: 'video', status: 'UNSUPPORTED_MAPPING', action: 'SKIP', reason: 'no exact task plugin binding with a usage profile', reason_code: 'NO_PLUGIN_USAGE_PROFILE', pricing_shape: 'video:video_second+video_tiers', pricing_scope: 'PLUGIN_OVERRIDE', plugin_key: 'alibaba', required_facts: '["seconds","resolution"]', available_facts: '[]', missing_facts: '["seconds","resolution"]', current_pricing: '{}', proposed_pricing: '{}', prices: '{"input_per_1m":{"unit":"token_per_1m","credits":"100","cost_cny":"1.6667","cost_usd":"0.25","selling_usd":"0.25","source_price_kind":"LIST_PRICE_WITH_VERIFIED_MULTIPLIER","promotion_multiplier":"0.3","promotion_rule_id":"dflop-gpt-text-2026-09","promotion_state":"VERIFIED","effective_credits":"30","effective_cost_cny":"0.5","effective_cost_usd":"0.075","effective_selling_usd":"0.075"}}', expression: '', delta_percent: '' }],
      },
    },
  })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={client}><DflopPricingSync /></QueryClientProvider>)

  await userEvent.setup().click(await screen.findByRole('button', { name: 'Sync Now / Preview' }))

  expect(await screen.findByText('video-per-second')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Apply selected prices' })).toBeDisabled()
  await userEvent.setup().click(screen.getByRole('button', { name: 'Details for video-per-second' }))
  expect(screen.getByRole('dialog')).toHaveTextContent('video:video_second+video_tiers')
  expect(screen.getByRole('dialog')).toHaveTextContent('NO_PLUGIN_USAGE_PROFILE')
  expect(screen.getByRole('dialog')).toHaveTextContent('No exact plugin usage profile')
  expect(screen.getByRole('dialog')).toHaveTextContent('seconds, resolution')
  expect(screen.getByRole('dialog')).toHaveTextContent('LIST_PRICE_WITH_VERIFIED_MULTIPLIER')
  expect(screen.getByRole('dialog')).toHaveTextContent('100 × 0.3 = 30 credits')
  client.clear()
})

it('shows source channel and catalog integrity without exposing its credential', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    let data: unknown = []
    if (url.endsWith('/config')) {
      data = {
        enabled: true, source_channel_id: 7, auto_sync_enabled: false, auto_apply_enabled: false,
        sync_interval_hours: 6, cny_to_usd: '0.15', markup_multiplier: '1', sync_text: true,
        sync_image: true, sync_video: true, sync_audio: true, sync_other: true,
        include_new_callable_models: true, max_auto_increase_percent: '20',
        max_auto_decrease_percent: '50', allow_auto_apply_new_models: false,
      }
    }
    if (url.endsWith('/source_channels')) {
      data = [{ id: 7, name: 'DFLOP channel', status: 1, base_url: 'https://api.dflop.top' }]
    }
    return { data: { success: true, data } }
  })
  vi.spyOn(api, 'post').mockResolvedValue({ data: { success: true, data: {
    run: { id: 'effective-preview', status: 'preview', source_hash: 'hash', pricing_version_before: 'version', started_at: Math.floor(Date.now() / 1000), changed_count: 0, blocked_count: 0,
      currency_snapshot: JSON.stringify({ source_mode: 'AUTHENTICATED_EFFECTIVE', source_channel: { id: 7, name: 'DFLOP channel', status: 1, base_url: 'https://api.dflop.top' }, catalog_schema_version: '1.0', effective_count: 212, callable_count: 212, public_count: 106, integrity: ['PUBLIC_CATALOG_ANOMALY'], effective: { etag: '"cat1"', fetched_at: 1, state: 'FRESH' } }) },
    items: [],
  } } })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={client}><DflopPricingSync /></QueryClientProvider>)
  expect(await screen.findByText('DFLOP channel')).toBeVisible()
  await userEvent.setup().click(screen.getByRole('button', { name: 'Sync Now / Preview' }))
  expect(await screen.findByText(/AUTHENTICATED_EFFECTIVE/)).toBeVisible()
  expect(screen.getByText(/PUBLIC_CATALOG_ANOMALY/)).toBeVisible()
  expect(screen.queryByText(/secret/i)).not.toBeInTheDocument()
  client.clear()
})

it('saves the selected DFLOP source channel ID without requesting its key', async () => {
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url.endsWith('/source_channels')) {
      return { data: { success: true, data: [{ id: 7, name: 'DFLOP channel', status: 1, base_url: 'https://api.dflop.top' }] } }
    }
    if (url.endsWith('/config')) {
      return { data: { success: true, data: {
      enabled: true, source_channel_id: 0, auto_sync_enabled: false, auto_apply_enabled: false,
      sync_interval_hours: 6, cny_to_usd: '0.15', markup_multiplier: '1', sync_text: true,
      sync_image: true, sync_video: true, sync_audio: true, sync_other: true,
      include_new_callable_models: true, max_auto_increase_percent: '20',
      max_auto_decrease_percent: '50', allow_auto_apply_new_models: false,
      } } }
    }
    return { data: { success: true, data: [] } }
  })
  const put = vi.spyOn(api, 'put').mockImplementation(async (_, value) => ({ data: { success: true, data: value } }))
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={client}><DflopPricingSync /></QueryClientProvider>)
  const user = userEvent.setup()
  await user.click(await screen.findByRole('combobox', { name: 'DFLOP Pricing Source Channel' }))
  await user.click(await screen.findByRole('option', { name: /DFLOP channel/ }))
  await user.click(screen.getByRole('button', { name: 'Save settings' }))
  expect(put).toHaveBeenCalledWith('/api/option/model_pricing/dflop/config', expect.objectContaining({ source_channel_id: 7 }))
  expect(get.mock.calls.every(([url]) => !url.includes('key'))).toBe(true)
  client.clear()
})

it('requires an explicit manual confirmation for a public-only catalog warning', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => ({ data: { success: true, data: url.endsWith('/config') ? {
    enabled: true, source_channel_id: 7, auto_sync_enabled: false, auto_apply_enabled: false,
    sync_interval_hours: 6, cny_to_usd: '0.15', markup_multiplier: '1', sync_text: true,
    sync_image: true, sync_video: true, sync_audio: true, sync_other: true,
    include_new_callable_models: true, max_auto_increase_percent: '20',
    max_auto_decrease_percent: '50', allow_auto_apply_new_models: false,
  } : [] } }))
  const post = vi.spyOn(api, 'post').mockImplementation(async (url) => ({ data: { success: true, data: url.endsWith('/preview') ? {
    run: { id: 'warning-preview', status: 'preview', trigger: 'manual', source_hash: 'effective-hash', pricing_version_before: 'version', started_at: Math.floor(Date.now() / 1000), changed_count: 1, blocked_count: 0,
      currency_snapshot: JSON.stringify({ source_mode: 'AUTHENTICATED_EFFECTIVE', integrity: ['PUBLIC_CATALOG_ANOMALY'], policy: { auto_block_reason: 'PUBLIC_CATALOG_ANOMALY', manual_confirmation_required: true } }) },
    items: [{ model_id: 'text', category: 'text', status: 'SUPPORTED_AUTO', action: 'UPDATE', reason: '', reason_code: '', pricing_scope: 'MODEL', current_pricing: '{}', proposed_pricing: '{}', prices: '{}', expression: 'tier("dflop", p * 1)', delta_percent: '' }],
  } : { id: 'warning-preview', status: 'applied' } } }))
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={client}><DflopPricingSync /></QueryClientProvider>)
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Sync Now / Preview' }))
  expect(await screen.findByText(/Production pricing source is healthy/)).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Apply selected prices' }))
  expect(screen.getByRole('alertdialog')).toHaveTextContent('The DFLOP public discovery catalog is inconsistent')
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await waitFor(() => expect(post).toHaveBeenCalledWith('/api/option/model_pricing/dflop/apply', expect.objectContaining({ acknowledge_public_catalog_anomaly: true, models: ['text'] })))
  client.clear()
})
