// Browser regression tests; Playwright is a development-only dependency.
const { test, before, after } = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');
const { chromium } = require('playwright');
let browser;
before(async () => {
  browser = await chromium.launch({ headless: true, ...(process.env.CHROME_PATH ? {executablePath:process.env.CHROME_PATH} : {}) });
});
after(async () => { await browser?.close(); });
async function fixture(tool = null) {
  const page = await browser.newPage();
  page.setDefaultTimeout(3000);
  await page.addInitScript(({tool}) => {
    const data = { version:'test',configDir:'/config',defaultWorkspaceDir:'/workspaces',providers:[
      {id:'p1',name:'One',kind:'openai',models:['model-a','model-b','model-c']},
      {id:'p2',name:'Two',kind:'gemini',models:['other-a','other-b']}
    ],tools:tool?[tool]:[],skills:[{id:'a'.repeat(32),name:'润色'},{id:'b'.repeat(32),name:'翻译'}] };
    window.saved = null; window.savedSkill = null; window.deletedSkill = null;
    window.go = {main:{App:{
      GetState:async()=>structuredClone(data), ListSessions:async()=>[],
      CreateTool:async input=>{window.saved=structuredClone(input);const saved={...input,id:'t1',path:'/workspaces/t1'};data.tools=[saved];return saved;},
      SaveTool:async input=>{window.saved=structuredClone(input);data.tools=[input];return input;},
      GetSkill:async id=>({...data.skills.find(skill=>skill.id===id),content:'仅润色文字'}),
      ChooseSkillImport:async()=>({id:'',name:'导入示例',content:'只整理用户提供的文本'}),
      SaveSkill:async input=>{window.savedSkill=structuredClone(input);const saved={...input,id:input.id||'c'.repeat(32)};data.skills=[...data.skills.filter(skill=>skill.id!==saved.id),saved];return saved;},
      DeleteSkill:async id=>{window.deletedSkill=id;data.skills=data.skills.filter(skill=>skill.id!==id);} 
    }}};
    window.runtime={EventsOn(){},ClipboardSetText:async()=>{}};
  },{tool});
  await page.goto(pathToFileURL(path.join(__dirname,'index.html')).href);
  await page.getByRole('button',{name:tool?'工具设置':'＋ 创建工具',exact:true}).click();
  return page;
}

test('model dropdown shows every configured model and preserves choice on save',async()=>{
 const page=await fixture();
 try {
  const model=page.getByLabel('模型 ID',{exact:true});
  assert.equal(await model.evaluate(n=>n.tagName),'SELECT');
  assert.equal(await model.inputValue(),'');
  assert.deepEqual(await model.locator('option').allTextContents(),['请选择模型','model-a','model-b','model-c','自定义模型 ID']);
  await model.selectOption('model-c');
  await page.getByLabel('工具名称',{exact:true}).fill('Test');
  await page.getByRole('button',{name:'创建工具',exact:true}).click();
  await page.waitForFunction(()=>window.saved!==null);
  assert.equal(await page.evaluate(()=>window.saved.model),'model-c');
 } finally {await page.close();}
});

test('editing an unlisted model retains its custom ID and provider changes clear selection',async()=>{
 const page=await fixture({id:'t1',name:'Test',description:'',system:'',providerId:'p1',model:'private-model',path:'/workspace',temperature:0.7,maxTokens:4096,historyTurns:10,sendTemperature:false,tokenParameter:'max_tokens'});
 try {
  assert.equal(await page.getByLabel('自定义模型 ID',{exact:true}).inputValue(),'private-model');
  await page.getByLabel('模型供应商',{exact:true}).selectOption('p2');
  const model=page.getByLabel('模型 ID',{exact:true});
  assert.equal(await model.inputValue(),'');
  assert.deepEqual(await model.locator('option').allTextContents(),['请选择模型','other-a','other-b','自定义模型 ID']);
 } finally {await page.close();}
});

test('reasoning settings and selected skill grants are saved independently',async()=>{
 const page=await fixture();
 try {
  await page.getByLabel('工具名称',{exact:true}).fill('Test');
  await page.getByText('请求参数与上下文',{exact:true}).click();
  const toggle=page.getByLabel('指定思考强度',{exact:true});
  assert.equal(await toggle.isChecked(),false);
  await toggle.check();
  assert.equal(await page.getByLabel('思考强度',{exact:true}).inputValue(),'medium');
  await page.getByLabel('思考强度',{exact:true}).selectOption('high');
  await page.getByLabel('润色',{exact:true}).check();
  assert.equal(await page.getByLabel('翻译',{exact:true}).isChecked(),false);
  assert.ok(await page.getByText('本版本仅支持纯文本 Skill，不支持非纯文本 Skill。',{exact:false}).isVisible());
  await page.getByRole('button',{name:'创建工具',exact:true}).click();
  await page.waitForFunction(()=>window.saved!==null);
  const saved=await page.evaluate(()=>window.saved);
  assert.equal(saved.sendReasoningEffort,true);
  assert.equal(saved.reasoningEffort,'high');
  assert.deepEqual(saved.skillIds,['a'.repeat(32)]);
 } finally {await page.close();}
});


test('library creation and import show the text-only warning and do not grant tools',async()=>{
 const page=await fixture();
 try {
  await page.getByRole('button',{name:'取消',exact:true}).click();
  await page.getByRole('button',{name:'技能库',exact:false}).click();
  assert.ok(await page.getByText('本版本仅支持纯文本 Skill，不支持非纯文本 Skill。',{exact:false}).isVisible());
  await page.getByRole('button',{name:'新建 Skill',exact:true}).click();
  assert.ok(await page.getByText('本版本仅支持纯文本 Skill，不支持非纯文本 Skill。',{exact:false}).isVisible());
  assert.match(await page.getByLabel('Skill 正文',{exact:true}).inputValue(),/本版本仅支持纯文本/);
  await page.getByLabel('Skill 名称',{exact:true}).fill('新技能');
  await page.getByLabel('Skill 正文',{exact:true}).fill('按用户要求整理文本');
  await page.getByRole('button',{name:'保存到技能库',exact:true}).click();
  await page.waitForFunction(()=>window.savedSkill!==null);
  assert.equal(await page.evaluate(()=>window.savedSkill.name),'新技能');
  await page.getByRole('button',{name:'关闭',exact:true}).click();
  await page.getByRole('button',{name:'＋ 创建工具',exact:true}).click();
  assert.equal(await page.getByLabel('新技能',{exact:true}).isChecked(),false);
  await page.getByRole('button',{name:'取消',exact:true}).click();
  await page.getByRole('button',{name:'技能库',exact:false}).click();
  await page.evaluate(()=>{window.savedSkill=null;});
  await page.getByRole('button',{name:'导入 Skill',exact:true}).click();
  assert.equal(await page.getByLabel('Skill 名称',{exact:true}).inputValue(),'导入示例');
  assert.equal(await page.evaluate(()=>window.savedSkill),null);
  await page.getByRole('button',{name:'保存到技能库',exact:true}).click();
  await page.waitForFunction(()=>window.savedSkill!==null);
  assert.equal(await page.evaluate(()=>window.savedSkill.content),'只整理用户提供的文本');
 } finally {await page.close();}
});

test('revoking a missing skill and disabling reasoning preserves the chosen model',async()=>{
 const page=await fixture({id:'t1',name:'Test',description:'',system:'',providerId:'p1',model:'model-b',path:'/workspace',temperature:0.7,maxTokens:4096,historyTurns:10,sendTemperature:false,tokenParameter:'max_tokens',sendReasoningEffort:true,reasoningEffort:'high',skillIds:['d'.repeat(32)]});
 try {
  await page.getByText('请求参数与上下文',{exact:true}).click();
  await page.getByLabel('指定思考强度',{exact:true}).uncheck();
  assert.equal(await page.getByLabel('思考强度',{exact:true}).isVisible(),false);
  await page.getByRole('checkbox',{name:/缺失的 Skill/}).uncheck();
  await page.getByRole('button',{name:'保存设置',exact:true}).click();
  await page.waitForFunction(()=>window.saved!==null);
  const saved=await page.evaluate(()=>window.saved);
  assert.equal(saved.sendReasoningEffort,false);
  assert.equal(saved.reasoningEffort,'high');
  assert.equal(saved.model,'model-b');
  assert.deepEqual(saved.skillIds,[]);
 } finally {await page.close();}
});
