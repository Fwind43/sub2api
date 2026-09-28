import { describe, expect, it } from 'vitest'
import { mergeUpstreamModelAliases as merge } from '../upstreamModelAliases'
import { buildModelMappingObject } from '@/composables/useModelWhitelist'

describe('upstream provider aliases', () => {
  it('strips the first provider segment and preserves the complete route', () => {
    const result = merge([' provider/model ', 'provider/model', 'bare', 'provider/org/model'], [], [])
    expect(result.models).toEqual(['model', 'bare', 'org/model'])
    expect(result.mappings).toEqual([{ from: 'model', to: 'provider/model' }, { from: 'org/model', to: 'provider/org/model' }])
    expect(buildModelMappingObject('combined', result.models, result.mappings)).toEqual({ model: 'provider/model', bare: 'bare', 'org/model': 'provider/org/model' })
  })
  it('retains full IDs for collisions and unprefixed upstream names', () => {
    expect(merge(['one/model', 'two/model', 'bare', 'one/bare'], [], []).models).toEqual(['one/model', 'two/model', 'bare', 'one/bare'])
  })
  it('preserves manual mappings and selected identities', () => {
    const manual = [{ from: 'model', to: 'custom/target' }]
    expect(merge(['provider/model'], ['model'], manual)).toEqual({ models: ['model', 'provider/model'], mappings: manual })
    expect(merge(['provider/model'], ['model'], []).models).toEqual(['model', 'provider/model'])
    expect(merge(['provider/model'], [], [{ from: 'provider/model', to: 'custom' }]).mappings).toEqual([{ from: 'provider/model', to: 'custom' }])
  })
  it('migrates identity entries and is idempotent', () => {
    const first = merge(['provider/model'], ['provider/model'], [{ from: 'provider/model', to: 'provider/model' }])
    expect(first).toEqual({ models: ['model'], mappings: [{ from: 'model', to: 'provider/model' }] })
    expect(merge(['provider/model'], first.models, first.mappings)).toEqual(first)
  })
  it('does not mutate caller arrays or override wildcard mappings', () => {
    const models = ['existing']; const mappings = [{ from: '*', to: 'custom' }]
    expect(merge(['provider/model'], models, mappings).models).toEqual(['existing', 'provider/model'])
    expect(models).toEqual(['existing']); expect(mappings).toEqual([{ from: '*', to: 'custom' }])
  })
  it('ignores empty IDs and leaves malformed or unprefixed IDs intact', () => {
    expect(merge(['', ' ', '/model', 'provider/', 'bare'], [], []).models).toEqual(['/model', 'provider/', 'bare'])
  })
})
