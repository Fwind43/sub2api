const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const assert = require('node:assert/strict');
const ts = require('typescript');
const {webcrypto} = require('node:crypto');
const box = {exports: {}, URL, Date, Uint8Array, crypto: webcrypto};
const source = fs.readFileSync(path.resolve(__dirname, '../src/utils/commandcodeAuthorization.ts'), 'utf8');
vm.runInNewContext(ts.transpileModule(source, {compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2020}}).outputText, box);
const {createCommandCodeAuthorizationSession:create,consumeCommandCodeAuthorizationResult:consume} = box.exports;
let passed = 0;
function check(name, fn){ fn(); passed++; console.log('PASS '+name); }
function result(s, patch={}) { return JSON.stringify({state:s.state,apiKey:'synthetic-test-key',userId:'synthetic-id',userName:'synthetic-user',keyName:'test',...patch}); }
check('cryptographic sessions differ',()=>{const a=create(0),b=create(0);assert.match(a.state,/^[a-f0-9]{64}$/);assert.notEqual(a.state,b.state);assert.equal(a.expiresAt,600000);assert.equal(a.callback,undefined);});
check('valid JSON returns credentials once',()=>{const s=create(0),r=consume(result(s),s,1);assert.equal(r.apiKey,'synthetic-test-key');assert.equal(r.userId,'synthetic-id');assert.equal(s.consumed,true);assert.throws(()=>consume(result(s),s,1),/used/);});
check('expired result rejected at boundary',()=>{const s=create(0);assert.throws(()=>consume(result(s),s,600000),/expired/);});
check('mismatched state does not consume session',()=>{const s=create(0);assert.throws(()=>consume(result(s,{state:'wrong'}),s,1),/state/);assert.equal(s.consumed,false);});
check('empty state rejected',()=>{const s=create(0);s.state='';assert.throws(()=>consume(result(s),s,1),/state/);});
check('denial does not expose upstream message',()=>{const s=create(0);assert.throws(()=>consume(result(s,{error:'sensitive'}),s,1),/^Error: denied$/);});
for(const apiKey of ['', 'bad\nkey','bad key','x'.repeat(16385),3,null])check('invalid credential rejected',()=>{const s=create(0);assert.throws(()=>consume(result(s,{apiKey}),s,1),/missingKey/);});
for(const input of ['null','[]','3','false','"string"','not-json','x'.repeat(32769),'http://127.0.0.1:18765/callback'])check('invalid input rejected',()=>{const s=create(0);assert.throws(()=>consume(input,s,1),/invalid/);});
for(const key of ['userId','userName','keyName'])check('invalid metadata rejected',()=>{const s=create(0);assert.throws(()=>consume(result(s,{[key]:{}}),s,1),/invalid/);});
console.log('TOTAL '+passed+' passed');
