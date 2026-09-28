import assert from 'node:assert/strict';
import { once } from 'node:events';
import { startReceiver } from '../public/commandcode-login.mjs';
const state = 'a'.repeat(64);
async function boot(ttl) {
  const results=[];const server=startReceiver(state,r=>results.push(r),ttl);
  await once(server,'listening');
  return {server,results,url:`http://127.0.0.1:${server.address().port}/callback`};
}
const body={state,apiKey:'synthetic-key',userId:'test',userName:'synthetic',keyName:'test'};
async function post(url,data,type='application/x-www-form-urlencoded') {
  return fetch(url,{method:'POST',headers:{'Content-Type':type},body:typeof data==='string'?data:new URLSearchParams(data).toString()});
}
const a=await boot();
try {
  assert.equal(a.server.address().address,'127.0.0.1');
  assert.equal((await fetch(a.url)).status,405);
  assert.equal((await post(a.url,{...body,state:'b'.repeat(64)})).status,403);
  assert.equal((await post(a.url,{...body,apiKey:'bad key'})).status,400);
  assert.equal((await post(a.url,new URLSearchParams(body).toString()+'&state='+state)).status,400);
  assert.equal((await post(a.url,'x'.repeat(32769))).status,413);
  assert.equal((await post(a.url,body,'text/plain')).status,415);
  assert.equal(a.results.length,0);
  const responses=await Promise.allSettled([post(a.url,body),post(a.url,body)]);
  assert.equal(responses.filter(r=>r.status==='fulfilled'&&r.value.status===200).length,1);
  assert.equal(a.results.length,1);assert.deepEqual(a.results[0],body);
  for(const r of responses) if(r.status==='fulfilled') {assert.equal(r.value.headers.get('cache-control'),'no-store');assert.ok(!(await r.value.text()).includes(body.apiKey));}
  console.log('PASS loopback, GET, state, invalid credential, duplicate, size, content type, form POST and replay');
} finally {a.server.close();a.server.closeAllConnections();}
const b=await boot();
try { assert.equal((await post(b.url,JSON.stringify(body),'application/json')).status,200);assert.equal(b.results.length,1);console.log('PASS JSON POST'); }
finally {b.server.close();b.server.closeAllConnections();}
const c=await boot(30);
await new Promise(r=>setTimeout(r,70));assert.equal(c.server.listening,false);assert.equal(c.results.length,0);console.log('PASS expiration closes listener');
