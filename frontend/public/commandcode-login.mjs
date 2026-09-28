#!/usr/bin/env node
// Run on the computer containing your browser, never on the remote server.
import http from 'node:http';
import { timingSafeEqual } from 'node:crypto';
import { pathToFileURL } from 'node:url';
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
      onResult({state, apiKey:body.apiKey,userId:body.userId || '',userName:body.userName || '',keyName:body.keyName || ''});
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
  if (!/^[a-f0-9]{64}$/.test(state || '')) {
    console.error('Usage: node commandcode-login.mjs <session-state>'); process.exitCode = 1;
  } else {
    console.error('Keep this terminal private. Credentials are printed only here and are not saved to disk.');
    const server = startReceiver(state, result => console.log(JSON.stringify(result)));
    server.on('error', () => { console.error('Unable to start callback receiver'); process.exitCode = 1; });
    server.on('listening', () => {
      const callback = `http://127.0.0.1:${server.address().port}/callback`;
      const url = new URL('https://commandcode.ai/studio/auth/cli');
      url.searchParams.set('callback', callback); url.searchParams.set('state', state); url.searchParams.set('mode', 'redirect');
      console.error('Open this URL in your local browser within 10 minutes:'); console.error(url.toString());
    });
  }
}
