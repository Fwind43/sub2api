#!/usr/bin/env node
// Run on the computer containing your browser, never on the remote server.
import http from 'node:http';
import { readFile } from 'node:fs/promises';
import { timingSafeEqual } from 'node:crypto';
import { pathToFileURL } from 'node:url';
// Optional local-only session archive. No cookies enter the callback result.
export async function enrichSubscription(result, sessionFile, request = fetch) {
  if (!sessionFile || !result.userId) return result;
  try {
    const archive = JSON.parse(await readFile(sessionFile, 'utf8'));
    const now = Date.now() / 1000;
    const url = new URL('https://api.commandcode.ai/internal/billing/subscriptions');
    const cookies = (Array.isArray(archive.cookies) ? archive.cookies : []).filter(c => {
      const domain = typeof c.domain === 'string' ? c.domain : '';
      const hostMatch = domain.startsWith('.')
        ? (url.hostname === domain.slice(1) || url.hostname.endsWith(domain))
        : url.hostname === domain;
      const path = c.path || '/';
      return hostMatch && (url.pathname === path || url.pathname.startsWith(path.endsWith('/') ? path : path + '/'))
        && (!(c.expires > 0) || c.expires > now)
        && typeof c.name === 'string' && /^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/.test(c.name)
        && typeof c.value === 'string' && !/[\x00-\x20\x7f;,]/.test(c.value);
    });
    if (!cookies.length) return result;
    const response = await request(url.toString(), {
      method: 'GET', redirect: 'error', signal: AbortSignal.timeout(10000),
      headers: {Cookie: cookies.map(c => c.name + '=' + c.value).join('; '), Accept: 'application/json'}
    });
    if (!response.ok) return result;
    const body = await response.json();
    const data = body?.data;
    if (body.success !== true || data?.userId !== result.userId || data.status !== 'active'
      || typeof data.planId !== 'string' || !/^[a-zA-Z0-9_-]{1,128}$/.test(data.planId)) return result;
    return {...result, planId: data.planId, subscriptionStatus: 'active', subscriptionUserId: data.userId};
  } catch { return result; } // Login remains usable if the optional session expired.
}
export function startReceiver(state, onResult, ttl = 600000) {
  if (!/^[a-f0-9]{64}$/.test(state)) throw new Error('Invalid state');
  let used = false;
  const deadline = Date.now() + ttl;
  const server = http.createServer(async (req, res) => {
    const send = (status, body) => {
      res.writeHead(status, {'Content-Type':'text/plain; charset=utf-8','Cache-Control':'no-store','Referrer-Policy':'no-referrer','X-Content-Type-Options':'nosniff'});
      res.end(body);
    };
    if (req.url !== '/callback') return send(404, 'Not found');
    if (req.method !== 'POST') return send(405, 'Authorization requires POST. Return to the official login page.');
    if (used || Date.now() >= deadline) return send(410, 'Session used or expired');
    const type = (req.headers['content-type'] || '').split(';')[0].trim().toLowerCase();
    if (!['application/json','application/x-www-form-urlencoded'].includes(type)) return send(415, 'Unsupported content type');
    try {
      const chunks = []; let size = 0;
      for await (const chunk of req) {
        size += chunk.length;
        if (size > 32768) {send(413, 'Request too large'); return;}
        chunks.push(chunk);
      }
      const text = Buffer.concat(chunks).toString('utf8');
      let body;
      if (type === 'application/json') body = JSON.parse(text);
      else {
        const params = new URLSearchParams(text);
        for (const key of params.keys()) if (params.getAll(key).length !== 1) return send(400, 'Duplicate field');
        body = Object.fromEntries(params);
      }
      if (!body || typeof body !== 'object' || Array.isArray(body)) return send(400, 'Invalid request');
      if (typeof body.state !== 'string' || !/^[a-f0-9]{64}$/.test(body.state) || !timingSafeEqual(Buffer.from(state), Buffer.from(body.state))) return send(403, 'Invalid state');
      if (used || Date.now() >= deadline) return send(410, 'Session used or expired');
      if (body.error) return send(400, 'Authorization denied');
      if (typeof body.apiKey !== 'string' || !body.apiKey || body.apiKey.length > 16384 || /[\x00-\x20\x7f]/.test(body.apiKey)) return send(400, 'Invalid credential');
      for (const key of ['userId','userName','keyName']) if (body[key] !== undefined && (typeof body[key] !== 'string' || body[key].length > 4096)) return send(400, 'Invalid metadata');
      used = true;
      await onResult({state, apiKey:body.apiKey,userId:body.userId || '',userName:body.userName || '',keyName:body.keyName || ''});
      send(200, 'Authorization received. Copy the JSON result from your terminal into sub2api. Do not share it.');
      clearTimeout(timer);
      server.close();
    } catch { if (!res.headersSent) send(400, 'Invalid request'); }
  });
  server.requestTimeout = 15000;
  server.headersTimeout = 10000;
  const timer = setTimeout(() => {server.close(); server.closeAllConnections();}, ttl);
  timer.unref();
  server.on('close', () => clearTimeout(timer));
  server.listen(0, '127.0.0.1');
  return server;
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const state = process.argv[2];
  const sessionFile = process.argv[3] === '--session-file' ? process.argv[4] : undefined;
  if (!/^[a-f0-9]{64}$/.test(state || '')) {
    console.error('Usage: node commandcode-login.mjs <session-state> [--session-file <local-storage-state.json>]'); process.exitCode = 1;
  } else {
    console.error('Keep this terminal private. Credentials are printed only here and are not saved to disk.');
    const server = startReceiver(state, async result => {
      const enriched = await enrichSubscription(result, sessionFile);
      if (sessionFile && !enriched.planId) console.error('Subscription unavailable or account mismatch; plan left unknown.');
      console.log(JSON.stringify(enriched));
    });
    server.on('error', () => { console.error('Unable to start callback receiver'); process.exitCode = 1; });
    server.on('listening', () => {
      const callback = `http://127.0.0.1:${server.address().port}/callback`;
      const url = new URL('https://commandcode.ai/studio/auth/cli');
      url.searchParams.set('callback', callback); url.searchParams.set('state', state); url.searchParams.set('mode', 'redirect');
      console.error('Open this URL in your local browser within 10 minutes:'); console.error(url.toString());
    });
  }
}
