import { describe, it, expect } from 'vitest'
import { consumeCommandCodeAuthorizationResult } from '../commandcodeAuthorization'

const state = 'a'.repeat(64)
const login = { state, apiKey: 'fixture-key', userId: 'fixture-user', userName: '', keyName: '' }
const subscription = { planId: 'individual-go', subscriptionStatus: 'active', subscriptionUserId: 'fixture-user' }
const session = () => ({ state, expiresAt: 2000, consumed: false })

describe('CommandCode subscription callback', () => {
  it('returns the matched active plan and strips unrelated session fields', () => {
    const pending = session()
    const result = consumeCommandCodeAuthorizationResult(JSON.stringify({ ...login, ...subscription, cookie: 'must-not-propagate' }), pending, 1000)
    expect(result).toEqual({ apiKey: 'fixture-key', userId: 'fixture-user', userName: '', keyName: '', planId: 'individual-go' })
    expect(pending.consumed).toBe(true)
  })
  it('preserves older callbacks without inventing a plan', () => {
    const result = consumeCommandCodeAuthorizationResult(JSON.stringify(login), session(), 1000)
    expect(result).not.toHaveProperty('planId')
  })
  it.each([
    { subscriptionUserId: 'another-user' },
    { subscriptionStatus: 'canceled' },
    { userId: '' },
    { planId: 'bad plan' },
    { planId: 123 },
  ])('rejects invalid subscription metadata: %j', invalid => {
    const pending = session()
    expect(() => consumeCommandCodeAuthorizationResult(JSON.stringify({ ...login, ...subscription, ...invalid }), pending, 1000)).toThrow('invalid')
    expect(pending.consumed).toBe(false)
  })
  it('does not reuse the callback', () => {
    const pending = session()
    const value = JSON.stringify({ ...login, ...subscription })
    consumeCommandCodeAuthorizationResult(value, pending, 1000)
    expect(() => consumeCommandCodeAuthorizationResult(value, pending, 1000)).toThrow('used')
  })
})
