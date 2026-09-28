const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const assert = require('node:assert/strict');
const ts = require('typescript');
const vue = require('vue/compiler-sfc');
const root = path.resolve(__dirname, '..');
const read = f => fs.readFileSync(path.join(root, f), 'utf8');
const exportsBox = { exports: {}, URL, Error };
vm.runInNewContext(ts.transpileModule(read('src/utils/commandcode.ts'), {compilerOptions: {module: ts.ModuleKind.CommonJS}}).outputText, exportsBox);
const normalize = exportsBox.exports.normalizeCommandCodeBaseUrl;
let passed = 0;
function check(name, fn) { fn(); passed++; console.log('PASS '+name); }
for (const [input, expected] of [
 [' https://gateway.example/v1/ ', 'https://gateway.example'],
 ['http://127.0.0.1:8317', 'http://127.0.0.1:8317'],
 ['https://gateway.example/proxy/v1', 'https://gateway.example/proxy'],
 ['https://gateway.example/proxy', 'https://gateway.example/proxy']
]) check('normalize '+input, () => assert.equal(normalize(input), expected));
for (const input of ['', 'nonsense', 'ftp://gateway.example', 'https://user:secret@gateway.example', 'https://gateway.example?key=test', 'https://gateway.example#token', 'https://api.openai.com/v1', 'https://COMMANDCODE.AI.', 'https://app.commandcode.ai', 'https://gateway.example/v1/responses', 'https://gateway.example/v1/chat/completions', 'https://gateway.example/v1/messages']) {
 check('reject '+input, () => assert.throws(() => normalize(input)));
}
const create = read('src/components/account/CreateAccountModal.vue');
const { descriptor } = vue.parse(create);
const ast = ts.createSourceFile('create.ts', descriptor.scriptSetup.content, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS);
function declaration(name) {
 for (const stmt of ast.statements) if(ts.isVariableStatement(stmt)) {
  for(const decl of stmt.declarationList.declarations) if(decl.name.getText(ast) === name) return stmt.getText(ast);
 }
 throw Error('Missing '+name);
}
const ref = value => ({value});
const state = {form:{platform:'openai'},accountCategory:ref('apikey'),commandCodePreset:ref(true),openaiPassthroughEnabled:ref(false),openaiAPIKeyResponsesWebSocketV2Mode:ref('shared'),openaiOAuthResponsesWebSocketV2Mode:ref('shared'),OPENAI_WS_MODE_OFF:'off',isOpenAIWSModeEnabled:v=>v!=='off',codexCLIOnlyEnabled:ref(false),openAICompactMode:ref('auto'),openAIResponsesMode:ref('auto')};
// Match upstream account defaults while retaining reactive capability behavior.
Object.assign(state, {
 openaiFlattenNamespacesEnabled:ref(false),
 openAILongContextBillingEnabled:ref(false),
 codexCLIOnlyAppServerEnabled:ref(false),
 codexFingerprintMode:ref('off'),
 openAIImagesUrlToB64JsonEnabled:ref(false),
 openAIEndpointCapabilities:ref(['chat_completions','embeddings']),
 openAITextGenerationCapabilityEnabled:{get value(){return state.openAIEndpointCapabilities.value.includes('chat_completions');}}
});
vm.createContext(state);
vm.runInContext(ts.transpileModule(declaration('buildOpenAIExtra')+'\nthis.build = buildOpenAIExtra;', {compilerOptions:{target:ts.ScriptTarget.ES2020}}).outputText,state);
check('gateway metadata forces passthrough and disables websockets',()=>{
 const value=state.build({custom:'preserved'});
 assert.equal(value.provider,'commandcode_gateway');assert.equal(value.openai_passthrough,true);assert.equal(value.openai_apikey_responses_websockets_v2_mode,'off');assert.equal(value.openai_apikey_responses_websockets_v2_enabled,false);assert.equal(value.custom,'preserved');
});
check('ordinary OpenAI account is not relabeled',()=>{state.commandCodePreset.value=false;assert.equal(state.build()?.provider,undefined)});
check('non-OpenAI metadata remains unchanged',()=>{state.form.platform='anthropic';const base={keep:true};assert.equal(state.build(base),base)});
for(const file of ['src/components/account/CreateAccountModal.vue','src/components/account/EditAccountModal.vue','src/components/common/PlatformTypeBadge.vue','src/views/admin/AccountsView.vue']) {
 check('Vue compiles '+file,()=>{const {descriptor,errors}=vue.parse(read(file),{filename:file});assert.equal(errors.length,0);vue.compileScript(descriptor,{id:'commandcode-test'});const result=vue.compileTemplate({source:descriptor.template.content,filename:file,id:'commandcode-test'});assert.deepEqual(result.errors,[])});
}
check('preset watcher is registered after form initialization',()=>assert.ok(create.indexOf('watch([() => form.platform, accountCategory]') > create.indexOf('const form = reactive(')));
const edit=read('src/components/account/EditAccountModal.vue');
check('edit validates gateway URL and preserves provider metadata',()=>{assert.ok(edit.includes('normalizeCommandCodeBaseUrl(editBaseUrl.value)'));assert.ok(edit.includes('const newExtra: Record<string, unknown> = { ...currentExtra }'));assert.ok(edit.includes("currentExtra.provider === 'commandcode_gateway'"))});
console.log(JSON.stringify({passed, failed:0}));
