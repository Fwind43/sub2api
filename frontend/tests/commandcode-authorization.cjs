const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const assert = require('node:assert/strict');
const ts = require('typescript');
const {webcrypto} = require('node:crypto');
const box = {exports: {}, URL, Date, Uint8Array, crypto: webcrypto};
const source = fs.readFileSync(path.resolve(__dirname, '../src/utils/commandcodeAuthorization.ts'), 'utf8');
vm.runInNewContext(ts.transpileModule(source, {compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2020}}).outputText, box);
const {createCommandCodeAuthorizationSession:create,commandCodeAuthorizationUrl:auth,consumeCommandCodeAuthorizationResult:consume} = box.exports;
let passed = 0;
function check(name, fn){ fn(); passed++; console.log('PASS '+name); }
function callback(s, patch={}) {const u=new URL(s.callback); for(const [k,v] of Object.entries({state:s.state,apiKey:'synthetic-test-key',userId:'synthetic-id',userName:'synthetic-user',keyName:'test',...patch})) u.searchParams.set(k,v); return u.toString();}
check('cryptographic sessions differ',()=>{const a=create(0),b=create(0); assert.match(a.state,/^[a-f0-9]{64}$/);assert.notEqual(a.state,b.state);assert.equal(a.expiresAt,600000);});
check('official CLI URL preserves callback state and mode',()=>{const s=create(0),u=new URL(auth(s));assert.equal(u.origin,'https://commandcode.ai');assert.equal(u.pathname,'/studio/auth/cli');assert.equal(u.searchParams.get('state'),s.state);assert.equal(u.searchParams.get('callback'),s.callback);assert.equal(u.searchParams.get('mode'),'redirect');});
check('valid callback returns credentials once',()=>{const s=create(0),u=callback(s),r=consume(u,s,1);assert.equal(r.apiKey,'synthetic-test-key');assert.equal(r.userId,'synthetic-id');assert.equal(r.userName,'synthetic-user');assert.equal(s.consumed,true);assert.throws(()=>consume(u,s,1),/used/);});
check('expired callback rejected at boundary',()=>{const s=create(0);assert.throws(()=>consume(callback(s),s,600000),/expired/);});
check('mismatched state rejected without consuming session',()=>{const s=create(0);assert.throws(()=>consume(callback(s,{state:'wrong'}),s,1),/state/);assert.equal(s.consumed,false);});
check('empty state rejected',()=>{const s=create(0);s.state='';assert.throws(()=>consume(callback(s),s,1),/state/);});
check('denial rejected without exposing upstream text',()=>{const s=create(0);assert.throws(()=>consume(callback(s,{error:'sensitive-upstream-message'}),s,1),/^Error: denied$/);});
for(const key of ['state','apiKey','userId','userName','keyName','error']) check('duplicate '+key+' rejected',()=>{const s=create(0),u=new URL(callback(s));u.searchParams.append(key,'first');u.searchParams.append(key,'second');assert.throws(()=>consume(u.toString(),s,1),/invalid/);});
for(const name of ['origin','path','userinfo','fragment','protocol'])check('invalid '+name+' rejected',()=>{const s=create(0),u=new URL(callback(s));if(name==='origin')u.hostname='example.invalid';if(name==='path')u.pathname='/other';if(name==='userinfo')u.username='user';if(name==='fragment')u.hash='secret';if(name==='protocol')u.protocol='https:';assert.throws(()=>consume(u.toString(),s,1),/invalid/);});
for(const key of ['', 'bad\nkey','bad key','x'.repeat(16385)])check('invalid credential rejected',()=>{const s=create(0);assert.throws(()=>consume(callback(s,{apiKey:key}),s,1),/missingKey/);});
check('malformed and oversized results rejected',()=>{const s=create(0);for(const u of ['not-a-url','x'.repeat(32769)])assert.throws(()=>consume(u,s,1),/invalid/);});
console.log('TOTAL '+passed+' passed');
