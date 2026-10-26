/* Run: node website/playground-engine/browser-smoke.cjs (requires Playwright + Chromium). */
const { chromium } = require('playwright');
const http = require('node:http');
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
const root = path.resolve(__dirname, '../../docs/playground');
const server = http.createServer((req,res) => {
 const name = new URL(req.url,'http://localhost').pathname;
 if (name === '/') { res.setHeader('content-type','text/html'); res.end('<!doctype html><title>Speedle engine smoke test</title>'); return; }
 const file = path.join(root,name);
 if (!file.startsWith(root+path.sep)) {res.writeHead(403).end();return;}
 fs.readFile(file,(error,bytes)=>{if(error){res.writeHead(404).end();return;}res.setHeader('content-type',name.endsWith('.js')?'text/javascript':name.endsWith('.wasm')?'application/wasm':'application/octet-stream');res.end(bytes);});
});
(async()=>{
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 const browser = await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH||'/usr/bin/chromium',args:['--no-sandbox']});
 try {
  const page = await browser.newPage();
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto(`http://127.0.0.1:${server.address().port}`);
  const results = await page.evaluate(async()=>{
   const engine = await import('/engine.js');
   const meta = await engine.initEngine();
   const request = {subject:{principals:[{type:'user',name:'alice'}]},serviceName:'playground',resource:'book',action:'read'};
   const cases=[
    ['allow','grant user alice read book',true],
    ['deny','grant user alice read book\ndeny user alice read book',false],
    ['no match','grant user bob read book',false],
    ['role','grant user alice reader on book\ngrant role reader read book',true],
    ['condition','grant user alice read book if request_user == \'alice\'',true],
    ['functions','grant user alice read book if Max(1,2,3) == 3',true],
    ['regexp','grant user alice read expr:^bo.*$',true],
    ['literal wildcard','grant user alice read *',false],
   ];
   const outputs=[];
   for (const [name,policies,want] of cases) outputs.push({name,want,result:await engine.evaluate({policies,request})});
   outputs.push({name:'typed numeric',want:true,result:await engine.evaluate({policies:'grant user alice read book if clearance >= 3',request:{...request,attributes:[{name:'clearance',type:'numeric',value:4}]}})});
   for(const policies of ['grant user','grant user alice read book trailing','grant user alice read book if unknown > 3','grant user alice read book if 5']) outputs.push({name:'error '+policies,error:true,result:await engine.evaluate({policies,request})});
   const many=await Promise.all(Array.from({length:10},(_,i)=>engine.evaluate({policies:i%2?'grant user bob read book':'grant user alice read book',request})));
   return {meta,outputs,many,globalBridge:typeof globalThis.evaluateSpeedle};
  });
  assert.equal(results.meta.revision,'7f61995d3344e94ed614494e013dacdcfa491b26');
  assert.equal(results.globalBridge,'function');
  for(const {name,want,error,result} of results.outputs){
   if(error){assert.equal(result.ok,false,name);assert.equal(Object.hasOwn(result,'allowed'),false,name);assert.ok(result.error,name);}
   else{assert.equal(result.ok,true,name+' '+JSON.stringify(result.error));assert.equal(result.allowed,want,name);}
   console.log('PASS',name);
  }
  results.many.forEach((result,i)=>assert.equal(result.allowed,i%2===0));
  assert.deepEqual(errors,[]);console.log('PASS 10 concurrent/repeated requests; no browser exceptions; global bridge available');
  console.log('ENGINE',results.meta);
 } finally { await browser.close(); server.close(); }
})().catch(error=>{console.error(error);server.close();process.exitCode=1;});
