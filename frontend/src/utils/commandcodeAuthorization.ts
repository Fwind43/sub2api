/** Credentials arrive by POST at the local receiver, never in the callback URL.
 * Pending sessions and credentials must not be persisted in browser storage.
 */
export interface CommandCodeAuthorizationSession {
  state: string
  expiresAt: number
  consumed: boolean
}
export interface CommandCodeAuthorizationResult {
  apiKey: string
  userId: string
  userName: string
  keyName: string
}
export const COMMANDCODE_AUTH_TTL_MS = 10 * 60 * 1000
export function createCommandCodeAuthorizationSession(now = Date.now()): CommandCodeAuthorizationSession {
  const bytes = new Uint8Array(32)
  globalThis.crypto.getRandomValues(bytes)
  return {
    state: Array.from(bytes, byte => byte.toString(16).padStart(2, '0')).join(''),
    expiresAt: now + COMMANDCODE_AUTH_TTL_MS,
    consumed: false
  }
}
export function consumeCommandCodeAuthorizationResult(
  input: string, session: CommandCodeAuthorizationSession, now = Date.now()
): CommandCodeAuthorizationResult {
  if (session.consumed) throw new Error('used')
  if (now >= session.expiresAt) throw new Error('expired')
  if (input.length > 32768) throw new Error('invalid')
  let body: Record<string, unknown>
  try { body = JSON.parse(input.trim()) } catch { throw new Error('invalid') }
  if (!body || typeof body !== 'object' || Array.isArray(body)) throw new Error('invalid')
  if (!session.state || body.state !== session.state) throw new Error('state')
  if ('error' in body) throw new Error('denied')
  const apiKey = body.apiKey
  if (typeof apiKey !== 'string' || !apiKey || apiKey.length > 16384 || /[\x00-\x20\x7f]/.test(apiKey)) throw new Error('missingKey')
  for (const key of ['userId', 'userName', 'keyName']) {
    if (body[key] !== undefined && (typeof body[key] !== 'string' || (body[key] as string).length > 4096)) throw new Error('invalid')
  }
  const result = {
    apiKey,
    userId: (body.userId as string | undefined) || '',
    userName: (body.userName as string | undefined) || '',
    keyName: (body.keyName as string | undefined) || ''
  }
  session.consumed = true
  return result
}
