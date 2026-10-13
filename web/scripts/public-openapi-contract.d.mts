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
export type CompatibilityChange = {
  kind: 'BREAKING' | 'POTENTIALLY_BREAKING' | 'ADDITIVE'
  at: string
  reason: string
}
export type PublicSnapshot = {
  version: number
  operations: Record<string, unknown>
}
export const methods: string[]
export function normalizePath(value: string): string
export function resolveSchema(
  value: unknown,
  spec: unknown,
  seen?: Set<string>
): unknown
export function validatePublicContract(
  spec: unknown,
  advertised: unknown,
  reference: unknown,
  root: string
): {
  operations: number
  duplicateOperationIds: number
  unresolvedRefs: number
  missingPublicOperations: number
  operationsWithoutBackend: number
  internalLeaks: number
}
export function contractSnapshot(spec: unknown): PublicSnapshot
export function compareContracts(
  before: PublicSnapshot,
  after: PublicSnapshot
): CompatibilityChange[]

export function parsePublicSpec(raw: string): unknown

export function readCommittedContract(
  root: string,
  baseRef?: string
): { commit: string; snapshot: PublicSnapshot }
