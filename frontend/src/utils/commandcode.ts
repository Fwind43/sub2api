/** CommandCode uses an explicitly configured compatible adapter, not its website. */
export function normalizeCommandCodeBaseUrl(input: string): string {
  const value = input.trim()
  if (!value) throw new Error('CommandCode adapter URL is required')
  let url: URL
  try { url = new URL(value) } catch { throw new Error('Enter a valid CommandCode adapter URL') }
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.search || url.hash) {
    throw new Error('Use an HTTP(S) adapter URL without credentials, query or fragment')
  }
  const host = url.hostname.toLowerCase().replace(/\.$/, '')
  if (host === 'api.openai.com' || host === 'commandcode.ai' || host.endsWith('.commandcode.ai')) {
    throw new Error('Use your compatible adapter URL, not the OpenAI API or CommandCode website')
  }
  const path = url.pathname.replace(/\/+$/, '')
  if (/\/(responses|chat\/completions|messages)$/i.test(path)) {
    throw new Error('Enter the adapter base URL, not a request endpoint')
  }
  url.pathname = path.replace(/\/v1$/i, '') || '/'
  return url.toString().replace(/\/+$/, '')
}
