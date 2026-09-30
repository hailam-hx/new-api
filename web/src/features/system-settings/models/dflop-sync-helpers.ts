export type DflopSelectionItem = {
  model_id: string
  status: string
  action: string
}

export function selectableModels(items: DflopSelectionItem[]): string[] {
  return items
    .filter(
      (item) =>
        item.status === 'SUPPORTED_AUTO' &&
        (item.action === 'ADD' || item.action === 'UPDATE')
    )
    .map((item) => item.model_id)
}

export function canApplyPreview(
  startedAtSeconds: number,
  nowSeconds: number,
  selected: string[]
): boolean {
  return (
    selected.length > 0 &&
    nowSeconds >= startedAtSeconds &&
    nowSeconds - startedAtSeconds < 600
  )
}
