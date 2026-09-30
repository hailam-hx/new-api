import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { t as translate } from 'i18next'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { invalidateModelPricing } from '@/features/model-pricing/api'
import { api } from '@/lib/api'
import { handleServerError } from '@/lib/handle-server-error'
import { createServerError } from '@/lib/server-error-message'

import { SettingsSection } from '../components/settings-section'
import { canApplyPreview, selectableModels } from './dflop-sync-helpers'

type Config = {
  enabled: boolean
  source_channel_id: number
  auto_sync_enabled: boolean
  auto_apply_enabled: boolean
  sync_interval_hours: number
  cny_to_usd: string
  markup_multiplier: string
  sync_text: boolean
  sync_image: boolean
  sync_video: boolean
  sync_audio: boolean
  sync_other: boolean
  include_new_callable_models: boolean
  max_auto_increase_percent: string
  max_auto_decrease_percent: string
  allow_auto_apply_new_models: boolean
}

type Price = {
  unit: string
  credits: string
  cost_cny: string
  cost_usd: string
  selling_usd: string
  source_price_kind?: string
  promotion_multiplier?: string
  promotion_rule_id?: string
  promotion_source?: string
  promotion_state?: string
  effective_credits?: string
  effective_cost_cny?: string
  effective_cost_usd?: string
  effective_selling_usd?: string
}

type Item = {
  model_id: string
  category: string
  status: string
  action: string
  reason: string
  current_pricing: string
  proposed_pricing: string
  prices: string
  expression: string
  delta_percent: string
  pricing_shape?: string
  pricing_scope?: string
  plugin_key?: string
  reason_code?: string
  required_facts?: string
  available_facts?: string
  missing_facts?: string
}

type Run = {
  id: string
  status: string
  trigger: string
  source_hash: string
  config_snapshot: string
  currency_snapshot?: string
  pricing_version_before: string
  started_at: number
  changed_count: number
  blocked_count: number
  error_message: string
}

type SourceChannel = { id: number; name: string; status: number; base_url: string }
type SourceRecord = {
  source_mode: string
  source_channel?: SourceChannel
  catalog_schema_version?: string
  effective_count?: number
  callable_count?: number
  public_count?: number
  public_hash?: string
  effective_hash?: string
  integrity?: string[]
  policy?: { manual_block_reason?: string; auto_block_reason?: string; manual_confirmation_required: boolean; conditions?: { code: string; severity: string }[] }
  effective?: { etag?: string; fetched_at?: number; state?: string }
}

function sourceRecord(run?: Run): SourceRecord | null {
  if (!run?.currency_snapshot) return null
  try {
    const parsed: unknown = JSON.parse(run.currency_snapshot)
    if (parsed && typeof parsed === 'object' && 'source_mode' in parsed && typeof parsed.source_mode === 'string') return parsed as SourceRecord
  } catch { /* Historical runs stored currency JSON directly. */ }
  return null
}

type Preview = { run: Run; items: Item[] }

const endpoint = '/api/option/model_pricing/dflop'

async function read<T>(path: string): Promise<T> {
  const response = await api.get(path)
  if (!response.data.success) throw createServerError(response.data, translate('Failed to fetch upstream prices'))
  return response.data.data as T
}

async function write<T>(path: string, body?: unknown, method: 'post' | 'put' = 'post'): Promise<T> {
  const response = await api[method](path, body)
  if (!response.data.success) throw createServerError(response.data, translate('Failed to fetch upstream prices'))
  return response.data.data as T
}

function priceRows(item: Item): [string, Price][] {
  try { return Object.entries(JSON.parse(item.prices) as Record<string, Price>) } catch { return [] }
}

function currentExpression(item: Item): string {
  try {
    const pricing = JSON.parse(item.current_pricing) as Record<string, unknown>
    const variants = pricing['billing_setting.plugin_billing_expr']
    if (item.plugin_key && variants && typeof variants === 'object' && !Array.isArray(variants)) {
      const expression = (variants as Record<string, unknown>)[item.plugin_key]
      if (typeof expression === 'string') return expression
    }
    for (const key of ['billing_setting.billing_expr', 'ModelPrice', 'ModelRatio']) {
      if (typeof pricing[key] === 'string') return pricing[key]
    }
    return '—'
  } catch { return '—' }
}

function factList(value?: string): string {
  if (!value) return '—'
  try {
    const facts: unknown = JSON.parse(value)
    if (Array.isArray(facts) && facts.every((fact) => typeof fact === 'string')) return facts.join(', ') || '—'
  } catch { /* Historical previews did not include fact metadata. */ }
  return '—'
}

export function DflopPricingSync() {
  const { t } = useTranslation()
  const statusLabels: Record<string, string> = {
    SUPPORTED_AUTO: t('Ready'),
    SUPPORTED_MANUAL: t('Needs review'),
    MANUAL_OVERRIDE: t('Manual override'),
    MANUAL_DRIFT: t('Manual drift'),
    UNSUPPORTED_MAPPING: t('Unsupported mapping'),
    SKIPPED_NON_CALLABLE: t('Not callable'),
    SKIPPED_SCOPE: t('Outside sync scope'),
    SKIPPED_NEW: t('New model excluded'),
    NOT_AVAILABLE_TO_SOURCE_KEY: t('Not available to source key'),
    PUBLIC_DIAGNOSTIC: t('Public diagnostic only'),
    preview: t('Preview'),
    applied: t('Applied'),
    rolled_back: t('Rolled back'),
    failed: t('Failed'),
  }
  const actionLabels: Record<string, string> = {
    ADD: t('Add'), UPDATE: t('Update'), UNCHANGED: t('Unchanged'),
    SKIP: t('Skip'), BLOCK: t('Blocked'),
  }
  const reasonLabels: Record<string, string> = {
    AMBIGUOUS_DISCOUNT: t('Discounted source price needs review'),
    PROMOTION_STATE_CHANGED: t('Promotion state changed'),
    MULTIMODAL_PROMOTION_MAPPING_REQUIRED: t('Other promoted billing features need mapping'),
    IMAGE_TIER_THRESHOLD_MISMATCH: t('Image size tier does not match plugin usage'),
    MISSING_SERVER_TOOL_USAGE: t('Server tool call usage is unavailable'),
    NO_PLUGIN_USAGE_PROFILE: t('No exact plugin usage profile'),
    UNKNOWN_CHARACTER_COUNT_SEMANTICS: t('Character counting rules are unverified'),
    UNVERIFIED_OUTPUT_COUNT: t('Successful image count is unverified'),
    UNVERIFIED_VIDEO_TOKEN_USAGE: t('Video token usage is unverified'),
    MISSING_USAGE_SECONDS: t('Billable seconds are unavailable'),
    MISSING_RESOLUTION_FACT: t('Output resolution is unavailable'),
    MISSING_USAGE_FACT: t('Required usage fact is unavailable'),
    INCOMPATIBLE_PLUGIN_SCHEMA: t('Plugin usage schema is incompatible'),
    PRICE_SHAPE_CHANGED: t('Pricing shape changed'),
    MODEL_COUNT_COLLAPSE: t('Catalog model count dropped sharply'),
    LIVE_CANARY_REQUIRED: t('A captured upstream usage result is required'),
    MISSING_CACHE_WRITE_MAPPING: t('Cache write pricing is incomplete'),
    MISSING_FAST_MODE_MAPPING: t('Fast mode and related pricing need mapping'),
    MISSING_IMAGE_TOKEN_MAPPING: t('Image pricing needs an exact runtime mapping'),
    NO_ASYNC_TTS_BINDING: t('No asynchronous speech task binding'),
    NO_EXACT_AVATAR_PLUGIN_BINDING: t('No exact avatar task binding'),
    NO_EXACT_FIXED_TASK_BINDING: t('No exact fixed task binding'),
    NO_EXACT_MUSIC_PLUGIN_BINDING: t('No exact music task binding'),
    NO_EXACT_VIDEO_PLUGIN_BINDING: t('No exact video task binding'),
    NO_EXACT_VOICE_CLONE_BINDING: t('No exact voice clone task binding'),
    NO_EXACT_IMAGE_PLUGIN_BINDING: t('No exact image task binding'),
  }
  const queryClient = useQueryClient()
  const configQuery = useQuery({ queryKey: ['dflop-sync-config'], queryFn: () => read<Config>(`${endpoint}/config`) })
  const channelsQuery = useQuery({ queryKey: ['dflop-sync-source-channels'], queryFn: () => read<SourceChannel[]>(`${endpoint}/source_channels`) })
  const historyQuery = useQuery({ queryKey: ['dflop-sync-history'], queryFn: () => read<Run[]>(`${endpoint}/runs`) })
  const [draftOverride, setDraftOverride] = useState<Config | null>(null)
  const draft = draftOverride ?? configQuery.data
  const [preview, setPreview] = useState<Preview | null>(null)
  const [selected, setSelected] = useState<string[]>([])
  const [search, setSearch] = useState('')
  const [confirmAuto, setConfirmAuto] = useState(false)
  const [confirmApply, setConfirmApply] = useState(false)
  const [rollbackID, setRollbackID] = useState<string | null>(null)
  const [detail, setDetail] = useState<Item | null>(null)
  const [now, setNow] = useState(() => Math.floor(Date.now() / 1000))

  useEffect(() => { const timer = window.setInterval(() => setNow(Math.floor(Date.now() / 1000)), 30_000); return () => window.clearInterval(timer) }, [])

  const saveConfig = useMutation({
    mutationFn: (value: Config) => write<Config>(`${endpoint}/config`, value, 'put'),
    onSuccess: (value) => { setDraftOverride(value); queryClient.invalidateQueries({ queryKey: ['dflop-sync-config'] }); setPreview(null); toast.success(t('DFLOP settings saved')) },
    onError: (error) => handleServerError(error),
  })
  const previewMutation = useMutation({
    mutationFn: () => write<Preview>(`${endpoint}/preview`),
    onSuccess: (value) => { setPreview(value); setSelected(selectableModels(value.items)); queryClient.invalidateQueries({ queryKey: ['dflop-sync-history'] }) },
    onError: (error) => handleServerError(error),
  })
  const applyMutation = useMutation({
    mutationFn: () => write<Run>(`${endpoint}/apply`, { preview_id: preview?.run.id, expected_version: preview?.run.pricing_version_before, models: selected, adopt: selected.some((name) => preview?.items.some((item) => item.model_id === name && (item.status === 'MANUAL_OVERRIDE' || item.status === 'MANUAL_DRIFT'))), acknowledge_public_catalog_anomaly: sourceRecord(preview?.run)?.policy?.manual_confirmation_required === true }),
    onSuccess: async () => { setConfirmApply(false); setPreview(null); await Promise.all([invalidateModelPricing(queryClient), queryClient.invalidateQueries({ queryKey: ['dflop-sync-history'] })]); toast.success(t('DFLOP prices applied')) },
    onError: (error) => { setConfirmApply(false); handleServerError(error); setPreview(null) },
  })
  const rollbackMutation = useMutation({
    mutationFn: (id: string) => write<Run>(`${endpoint}/runs/${encodeURIComponent(id)}/rollback`),
    onSuccess: async () => { setRollbackID(null); setPreview(null); await Promise.all([invalidateModelPricing(queryClient), queryClient.invalidateQueries({ queryKey: ['dflop-sync-history'] })]); toast.success(t('DFLOP pricing rollback completed')) },
    onError: (error) => { setRollbackID(null); handleServerError(error) },
  })
  const visible = useMemo(() => preview?.items.filter((item) => item.model_id.toLowerCase().includes(search.toLowerCase())) ?? [], [preview, search])
  const previewMarkup = preview?.run.config_snapshot ? (JSON.parse(preview.run.config_snapshot) as Config).markup_multiplier : '—'
  const source = sourceRecord(preview?.run)
  const active = preview?.run.status === 'preview' && source?.source_mode === 'AUTHENTICATED_EFFECTIVE' && source.policy !== undefined && !source.policy.manual_block_reason && canApplyPreview(preview.run.started_at, now, selected)
  const update = <K extends keyof Config>(key: K, value: Config[K]) => setDraftOverride((old) => {
    const current = old ?? configQuery.data
    return current ? { ...current, [key]: value } : null
  })

  if (configQuery.isPending) return <LoadingState />
  if (configQuery.isError || !draft) return <ErrorState description={t('Failed to load DFLOP settings')} />

  const numericFields: { key: keyof Config; label: string; step: string }[] = [
    { key: 'sync_interval_hours', label: 'Sync interval (hours)', step: '1' },
    { key: 'cny_to_usd', label: 'CNY to USD', step: 'any' },
    { key: 'markup_multiplier', label: 'Selling markup', step: 'any' },
    { key: 'max_auto_increase_percent', label: 'Maximum automatic increase (%)', step: 'any' },
    { key: 'max_auto_decrease_percent', label: 'Maximum automatic decrease (%)', step: 'any' },
  ]
  const toggles: { key: keyof Config; label: string }[] = [
    { key: 'enabled', label: 'Enable DFLOP pricing sync' },
    { key: 'auto_sync_enabled', label: 'Automatic fetch and preview' },
    { key: 'auto_apply_enabled', label: 'Automatic apply' },
    { key: 'sync_text', label: 'Text models' },
    { key: 'sync_image', label: 'Image models' },
    { key: 'sync_video', label: 'Video models' },
    { key: 'sync_audio', label: 'Audio models' },
    { key: 'sync_other', label: 'Other models' },
    { key: 'include_new_callable_models', label: 'Include new callable models' },
    { key: 'allow_auto_apply_new_models', label: 'Automatically apply new models' },
  ]

  let detailUsageSource = t('Not established')
  if (detail?.status === 'SUPPORTED_AUTO') {
    detailUsageSource = detail.pricing_scope === 'PLUGIN_OVERRIDE' ? t('Task plugin final result') : t('Final relay response or request count')
  }
  const includesManualAdoption = selected.some((name) => preview?.items.some((item) => item.model_id === name && (item.status === 'MANUAL_OVERRIDE' || item.status === 'MANUAL_DRIFT')))
  let applyDescription = t('Review selected models before changing customer billing prices.')
  if (includesManualAdoption) {
	applyDescription = t('Selected manual prices will be adopted into DFLOP sync.')
  }
  if (source?.policy?.manual_confirmation_required) {
    applyDescription = t('The DFLOP public discovery catalog is inconsistent. These prices come from the healthy authenticated catalog for the selected channel. Automatic apply remains blocked. Confirm this manual change.')
    if (includesManualAdoption) applyDescription += ` ${t('Selected manual prices will be adopted into DFLOP sync.')}`
  }

  return <SettingsSection title={t('DFLOP Pricing Sync')}>
    <p className='text-muted-foreground text-sm'>{t('Preview DFLOP upstream costs before changing customer billing prices.')}</p>
    <div className='space-y-2'>
      <label className='block text-sm' htmlFor='dflop-source-channel'>{t('DFLOP Pricing Source Channel')}</label>
      <Select
        items={[{ value: 'none', label: t('No source channel') }, ...(channelsQuery.data ?? []).map((channel) => ({ value: String(channel.id), label: `${channel.name} · ${channel.base_url}` }))]}
        value={draft.source_channel_id ? String(draft.source_channel_id) : 'none'}
        onValueChange={(value) => update('source_channel_id', value === 'none' || value === null ? 0 : Number(value))}
      >
        <SelectTrigger id='dflop-source-channel' className='w-full max-w-lg'><SelectValue /></SelectTrigger>
        <SelectContent alignItemWithTrigger={false}><SelectGroup>
          <SelectItem value='none'>{t('No source channel')}</SelectItem>
          {(channelsQuery.data ?? []).map((channel) => <SelectItem key={channel.id} value={String(channel.id)} disabled={channel.status !== 1}>
            {channel.name} · {channel.base_url} · {channel.status === 1 ? t('Enabled') : t('Disabled')}
          </SelectItem>)}
        </SelectGroup></SelectContent>
      </Select>
      {channelsQuery.data?.find((channel) => channel.id === draft.source_channel_id) && <p className='text-muted-foreground text-sm'>
        <span>{channelsQuery.data.find((channel) => channel.id === draft.source_channel_id)?.name}</span> · {channelsQuery.data.find((channel) => channel.id === draft.source_channel_id)?.base_url}
      </p>}
    </div>
    <div className='grid gap-3 sm:grid-cols-2'>
      {toggles.map(({ key, label }) => <label key={key} className='flex items-center justify-between gap-2 rounded-md border p-3 text-sm'>
        <span>{t(label)}</span><Switch checked={Boolean(draft[key])} onCheckedChange={(checked) => update(key, checked as Config[typeof key])} />
      </label>)}
      {numericFields.map(({ key, label, step }) => <label key={key} className='flex flex-col gap-1 text-sm'>
        <span>{t(label)}</span><Input type='number' min={key === 'sync_interval_hours' ? 1 : 0} step={step} value={String(draft[key])} onChange={(event) => update(key, (key === 'sync_interval_hours' ? Number(event.target.value) : event.target.value) as Config[typeof key])} />
      </label>)}
    </div>
    {draft.auto_apply_enabled && <p role='alert' className='text-destructive text-sm'>{t('Automatic apply will update customer billing prices when DFLOP prices change, subject to configured safety limits.')}</p>}
    <div className='flex flex-wrap gap-2'>
      <Button disabled={saveConfig.isPending} onClick={() => { if (draft.auto_apply_enabled && !configQuery.data?.auto_apply_enabled) setConfirmAuto(true); else saveConfig.mutate(draft) }}>{t('Save settings')}</Button>
      <Button variant='outline' disabled={!configQuery.data?.enabled || previewMutation.isPending} onClick={() => previewMutation.mutate()}>{t('Sync Now / Preview')}</Button>
    </div>
    {preview && <div className='space-y-3'>
      {source && <div className='rounded-md border p-3 text-sm space-y-1'>
        <p>{t('Source mode')}: {source.source_mode}</p>
        {source.source_mode === 'AUTHENTICATED_EFFECTIVE' && <p>{t('Source: DFLOP authenticated catalog. Account and platform discounts are already included.')}</p>}
        <p>{t('Source channel')}: {source.source_channel ? `${source.source_channel.name} (#${source.source_channel.id}) · ${source.source_channel.base_url} · ${source.source_channel.status === 1 ? t('Enabled') : t('Disabled')}` : '—'}</p>
        <p>{t('Catalog schema')}: {source.catalog_schema_version || '—'} · {t('Effective catalog models')}: {source.effective_count ?? '—'} · {t('Public catalog models')}: {source.public_count ?? '—'}</p>
        <p>{t('Available to source key')}: {source.callable_count ?? '—'}</p>
        <p>{t('Effective source hash')}: <code className='break-all'>{source.effective_hash || '—'}</code></p>
        <p>{t('Public source hash')}: <code className='break-all'>{source.public_hash || '—'}</code></p>
        <p>{t('ETag')}: {source.effective?.etag || '—'} · {t('Last successful fetch')}: {source.effective?.fetched_at ? new Date(source.effective.fetched_at * 1000).toISOString() : '—'}</p>
        <p role={source.integrity?.some((state) => state !== 'HEALTHY') ? 'alert' : undefined}>{t('Integrity status')}: {source.policy?.conditions?.map((condition) => `${condition.code} (${condition.severity})`).join(', ') || source.integrity?.join(', ') || '—'}</p>
        {source.policy?.manual_confirmation_required && !source.policy.manual_block_reason && <p role='alert' className='text-amber-700 dark:text-amber-400'>{t('Production pricing source is healthy. The public discovery catalog is inconsistent. Manual apply requires confirmation; automatic apply is blocked.')}</p>}
        {source.policy?.manual_block_reason && <p role='alert' className='text-destructive'>{t('Manual and automatic apply are blocked')}: {source.policy.manual_block_reason}</p>}
        {source.public_count !== undefined && source.effective_count !== undefined && source.public_count > source.effective_count && <p>{t('This DFLOP key can access {{count}} models. Only models available to this source key can be synchronized.', { count: source.callable_count ?? source.effective_count })}</p>}
      </div>}
      <div className='text-sm'>{t('Source hash')}: <code>{preview.run.source_hash}</code> · {t('Changes')}: {preview.run.changed_count} · {t('Blocked')}: {preview.run.blocked_count}</div>
      {preview.run.error_message && <p role='alert' className='text-destructive text-sm'>{preview.run.error_message}</p>}
      <Input aria-label={t('Search models')} placeholder={t('Search models')} value={search} onChange={(event) => setSearch(event.target.value)} />
      <StaticDataTable data={visible} getRowKey={(item) => item.model_id} className='max-h-[35rem] overflow-auto' tableClassName='min-w-[1100px]' columns={[
        { id: 'select', header: t('Select'), cell: (item) => <Checkbox aria-label={t('Select {{model}}', { model: item.model_id })} checked={selected.includes(item.model_id)} disabled={item.action !== 'ADD' && item.action !== 'UPDATE' || !['SUPPORTED_AUTO', 'SUPPORTED_MANUAL', 'MANUAL_OVERRIDE', 'MANUAL_DRIFT'].includes(item.status)} onCheckedChange={(checked) => setSelected((old) => checked ? [...old, item.model_id] : old.filter((name) => name !== item.model_id))} /> },
        { id: 'model', header: t('Model'), cell: (item) => <span className='font-mono text-xs'>{item.model_id}</span> },
        { id: 'type', header: t('Type'), cell: (item) => item.category },
        { id: 'status', header: t('Status'), cell: (item) => <span title={t(item.reason)}>{statusLabels[item.status] ?? item.status} / {actionLabels[item.action] ?? item.action}</span> },
        { id: 'scope', header: t('Pricing scope'), cell: (item) => item.pricing_scope === 'PLUGIN_OVERRIDE' ? t('Provider override') : t('Model') },
        { id: 'details', header: t('Details'), cell: (item) => <Button variant='link' size='sm' onClick={() => setDetail(item)} aria-label={t('Details for {{model}}', { model: item.model_id })}>{t('Details')}</Button> },
        { id: 'reason', header: t('Reason'), cell: (item) => t(item.reason) || '—' },
        { id: 'current', header: t('Current Billing'), cell: (item) => <code className='block max-w-52 truncate' title={currentExpression(item)}>{currentExpression(item)}</code> },
        { id: 'raw', header: t('DFLOP raw price'), cell: (item) => priceRows(item).map(([name, price]) => `${name}: ${price.credits} ${price.unit}`).join(', ') || '—' },
        { id: 'cny', header: t('Upstream cost CNY'), cell: (item) => priceRows(item).map(([name, price]) => `${name}: ${price.cost_cny}`).join(', ') || '—' },
        { id: 'usd', header: t('Upstream cost USD'), cell: (item) => priceRows(item).map(([name, price]) => `${name}: ${price.cost_usd}`).join(', ') || '—' },
        { id: 'markup', header: t('Selling markup'), cell: () => previewMarkup },
        { id: 'selling', header: t('Proposed selling price'), cell: (item) => priceRows(item).map(([name, price]) => `${name}: ${price.selling_usd}`).join(', ') || '—' },
        { id: 'delta', header: t('Delta %'), cell: (item) => item.delta_percent || '—' },
      ]} />
      <Button disabled={!active} onClick={() => setConfirmApply(true)}>{t('Apply selected prices')}</Button>
      {preview.run.status === 'preview' && !active && <p className='text-muted-foreground text-sm'>{t('Select eligible models and refresh an expired preview before applying.')}</p>}
    </div>}
    <Dialog open={detail !== null} onOpenChange={(open) => { if (!open) setDetail(null) }} title={detail?.model_id ?? t('Details')} contentClassName='sm:max-w-xl'>
      {detail && <dl className='grid gap-3 text-sm sm:grid-cols-[10rem_1fr]'>
        <dt>{t('Pricing scope')}</dt><dd>{detail.pricing_scope === 'PLUGIN_OVERRIDE' ? t('Provider override') : t('Model')}</dd>
        <dt>{t('Candidate plugin')}</dt><dd>{detail.plugin_key || '—'}</dd>
        <dt>{t('Pricing shape')}</dt><dd className='break-all font-mono'>{detail.pricing_shape || '—'}</dd>
        <dt>{t('Billing features')}</dt><dd>{detail.pricing_shape?.split(':')[1]?.split('+').join(', ') || '—'}</dd>
        <dt>{t('Price source semantics')}</dt><dd>{priceRows(detail)[0]?.[1].source_price_kind || '—'}</dd>
        {source?.source_mode !== 'AUTHENTICATED_EFFECTIVE' && <><dt>{t('Promotion rule')}</dt><dd>{priceRows(detail)[0]?.[1].promotion_rule_id || '—'} · {priceRows(detail)[0]?.[1].promotion_multiplier || '—'} · {priceRows(detail)[0]?.[1].promotion_state || '—'}</dd></>}
        <dt>{t('Effective upstream cost')}</dt><dd>{priceRows(detail).map(([name, price]) => {
          if (source?.source_mode === 'AUTHENTICATED_EFFECTIVE') return `${name}: ${price.credits} points · ¥${price.cost_cny} · $${price.cost_usd} · ${t('Selling markup')} ${previewMarkup}: $${price.selling_usd}`
          if (price.effective_credits) return `${name}: ${price.credits} × ${price.promotion_multiplier || '1'} = ${price.effective_credits} credits · ¥${price.effective_cost_cny} · $${price.effective_cost_usd} · ${t('Selling markup')} ${previewMarkup}: $${price.effective_selling_usd}`
          return `${name}: —`
        }).join('; ')}</dd>
        <dt>{t('Required usage facts')}</dt><dd>{factList(detail.required_facts)}</dd>
        <dt>{t('Available usage facts')}</dt><dd>{factList(detail.available_facts)}</dd>
        <dt>{t('Missing usage facts')}</dt><dd>{factList(detail.missing_facts)}</dd>
        <dt>{t('Usage source')}</dt><dd>{detailUsageSource}</dd>
        <dt>{t('Evidence status')}</dt><dd>{detail.status === 'SUPPORTED_AUTO' ? t('Price, semantics, quantity and settlement verified') : t('Automatic settlement is not verified')}</dd>
        <dt>{t('Reason code')}</dt><dd className='font-mono'>{detail.reason_code || '—'}</dd>
        <dt>{t('Reason')}</dt><dd>{reasonLabels[detail.reason_code ?? ''] || (detail.reason ? t(detail.reason) : '—')}</dd>
        <dt>{t('Proposed expression')}</dt><dd className='break-all font-mono'>{detail.expression || '—'}</dd>
      </dl>}
    </Dialog>
    <div className='space-y-2'><h3 className='font-medium'>{t('Sync history')}</h3>
      {historyQuery.isError && <ErrorState description={t('Failed to load sync history')} />}
      <StaticDataTable data={historyQuery.data ?? []} getRowKey={(run) => run.id} columns={[
        { id: 'id', header: t('Run'), cell: (run) => <Button variant='link' onClick={async () => { try { setPreview(await read<Preview>(`${endpoint}/runs/${encodeURIComponent(run.id)}`)); setSelected([]) } catch (error) { handleServerError(error) } }}>{run.id}</Button> },
        { id: 'status', header: t('Status'), cell: (run) => statusLabels[run.status] ?? run.status },
        { id: 'changes', header: t('Changes'), cell: (run) => run.changed_count },
        { id: 'rollback', header: t('Rollback'), cell: (run) => run.status === 'applied' ? <Button variant='outline' size='sm' onClick={() => setRollbackID(run.id)}>{t('Rollback')}</Button> : null },
      ]} />
    </div>
    <ConfirmDialog open={confirmAuto} onOpenChange={setConfirmAuto} title={t('Enable automatic apply?')} desc={t('Automatic apply will update customer billing prices when DFLOP prices change, subject to configured safety limits.')} handleConfirm={() => { setConfirmAuto(false); saveConfig.mutate(draft) }} />
    <ConfirmDialog open={confirmApply} onOpenChange={setConfirmApply} title={t('Apply DFLOP prices?')} desc={applyDescription} disabled={!active} isLoading={applyMutation.isPending} handleConfirm={() => applyMutation.mutate()} />
    <ConfirmDialog open={Boolean(rollbackID)} onOpenChange={(open) => { if (!open) setRollbackID(null) }} title={t('Rollback DFLOP prices?')} desc={t('Rollback will restore pricing only if it has not changed since this run.')} isLoading={rollbackMutation.isPending} handleConfirm={() => { if (rollbackID) rollbackMutation.mutate(rollbackID) }} />
  </SettingsSection>
}
