/** CommandCode CLI authorization, not authorization-code/refresh-token OAuth.
 * A loopback redirect can be pasted back when sub2api runs on a remote host.
 * Credentials and pending sessions must never be persisted in browser storage.
 */
export interface CommandCodeAuthorizationSession {
  state: string
  callback: string
  expiresAt: number
  consumed: boolean
}
export interface CommandCodeAuthorizationResult {
  apiKey: string
  userId: string
  userName: string
  keyName: string
}
export const COMMANDCODE_AUTH_URL = "https://commandcode.ai/studio/auth/cli"
export const COMMANDCODE_AUTH_TTL_MS = 10 * 60 * 1000
export function createCommandCodeAuthorizationSession(now = Date.now()): CommandCodeAuthorizationSession {
  const bytes = new Uint8Array(32)
  globalThis.crypto.getRandomValues(bytes)
  return {
    state: Array.from(bytes, byte => byte.toString(16).padStart(2, '0')).join(''),
    callback: 'http://127.0.0.1:18765/callback',
    expiresAt: now + COMMANDCODE_AUTH_TTL_MS,
    consumed: false
  }
}
export function commandCodeAuthorizationUrl(session: CommandCodeAuthorizationSession): string {
  const url = new URL(COMMANDCODE_AUTH_URL)
  url.searchParams.set('callback', session.callback)
  url.searchParams.set('state', session.state)
  url.searchParams.set('mode', 'redirect')
  return url.toString()
}
export function consumeCommandCodeAuthorizationResult(
  input: string, session: CommandCodeAuthorizationSession, now = Date.now()
): CommandCodeAuthorizationResult {
  if (session.consumed) throw new Error('used')
  if (now >= session.expiresAt) throw new Error('expired')
  if (input.length > 32768) throw new Error('invalid')
  let url: URL
  try { url = new URL(input.trim()) } catch { throw new Error('invalid') }
  const callback = new URL(session.callback)
  if (url.origin !== callback.origin || url.pathname !== callback.pathname || url.username || url.password || url.hash) {
    throw new Error('invalid')
  }
  const query = url.searchParams
  for (const key of ['state', 'apiKey', 'userId', 'userName', 'keyName', 'error']) {
    if (query.getAll(key).length > 1) throw new Error('invalid')
  }
  if (!session.state || query.get('state') !== session.state) throw new Error('state')
  if (query.has('error')) throw new Error('denied')
  const apiKey = query.get('apiKey')?.trim() || ''
  if (!apiKey || apiKey.length > 16384 || /[\x00-\x20\x7f]/.test(apiKey)) throw new Error('missingKey')
  const result = {
    apiKey,
    userId: query.get('userId') || '',
    userName: query.get('userName') || '',
    keyName: query.get('keyName') || ''
  }
  session.consumed = true
  return result
}
