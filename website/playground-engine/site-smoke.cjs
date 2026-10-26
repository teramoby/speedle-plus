/* Full-site real-browser integration. Requires Playwright + Chromium.
   Run from the repo root: node website/playground-engine/site-smoke.cjs
   Optional CHROMIUM_PATH and SCREENSHOT_DIR environment variables. */
const {chromium} = require('playwright');
const http = require('node:http');
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
const root = path.resolve(__dirname, '../../docs');
const server = http.createServer((req,res)=>{
  let url;
  try {url=decodeURIComponent(new URL(req.url,'http://localhost').pathname)} catch {res.writeHead(400).end();return;}
  let file=path.resolve(root,'.'+url);
  if(!file.startsWith(root+path.sep) && file!==root){res.writeHead(403).end();return;}
  if(fs.existsSync(file)&&fs.statSync(file).isDirectory())file=path.join(file,'index.html');
  const type={'.html':'text/html; charset=utf-8','.css':'text/css','.js':'text/javascript','.wasm':'application/wasm','.svg':'image/svg+xml','.txt':'text/plain'}[path.extname(file)] || 'application/octet-stream';
  fs.readFile(file,(error,bytes)=>{if(error){res.writeHead(404).end();return;}res.setHeader('content-type',type);res.end(bytes);});
});
(async()=>{
  await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
  let browser;
  try {
    browser=await chromium.launch({headless:true,...(process.env.CHROMIUM_PATH?{executablePath:process.env.CHROMIUM_PATH}:{})});
    const page=await browser.newPage({viewport:{width:1440,height:1100}});
    const origin=`http://127.0.0.1:${server.address().port}`;
    const errors=[],requests=[];
    page.on('pageerror',e=>errors.push(e.message));
    page.on('request',r=>requests.push({url:r.url(),method:r.method()}));
    await page.goto(origin+'/');
    await page.locator('.nav-links a[href="/playground/"]').click();
    assert.equal(new URL(page.url()).pathname,'/playground/');
    await page.waitForFunction(()=>document.getElementById('engine-text').textContent.includes('Ready'),{},{timeout:60000});
    for(const [sample,want] of [['allow','true'],['deny','false'],['nomatch','false'],['role','true'],['condition','true']]) {
      await page.locator(`[data-sample="${sample}"]`).click();
      await page.locator('#evaluate').click();
      await page.waitForFunction(want=>document.getElementById('result-label').textContent===want,want);
      const result=JSON.parse(await page.locator('#result-json').textContent());
      assert.equal(result.allowed,want==='true',sample); assert.equal(result.ok,true,sample);
    }
    await page.locator('#policies').fill('grant user alice read book trailing');
    await page.locator('#evaluate').click();
    await page.waitForFunction(()=>document.getElementById('result-card').classList.contains('error'));
    const invalid=JSON.parse(await page.locator('#result-json').textContent());
    assert.equal(invalid.ok,false); assert.equal(Object.hasOwn(invalid,'allowed'),false);
    await page.locator('#reset-button').click();
    await page.locator('#resource').fill('');
    await page.locator('#evaluate').click();
    assert.equal(await page.locator('#resource').getAttribute('aria-invalid'),'true');
    await page.locator('#resource').fill('book');
    assert.equal(await page.locator('#result-meta').textContent(),'Run again');
    await page.locator('#add-principal').click();
    assert.equal(await page.locator('[data-principal-index]').count(),2);
    await page.locator('[data-action="remove-principal"]').last().click();
    assert.equal(await page.locator('[data-principal-index]').count(),1);
    await page.locator('#add-attribute').click();
    await page.locator('[data-field="attributes.0.name"]').fill('clearance');
    await page.locator('[data-field="attributes.0.type"]').selectOption('numeric');
    await page.locator('[data-field="attributes.0.values.0"]').fill('4');
    await page.locator('#policies').fill('grant user alice read book if clearance >= 3');
    await page.locator('#evaluate').click();
    await page.waitForFunction(()=>document.getElementById('result-label').textContent==='true');
    await page.locator('[data-multiple="0"]').check();
    await page.locator('[data-action="add-value"]').click();
    assert.equal(await page.locator('[data-attribute-value]').count(),2);
    await page.locator('[data-action="remove-value"]').last().click();
    await page.locator('[data-multiple="0"]').uncheck();
    await page.locator('#reset-button').click();
    assert.equal(await page.locator('[data-attribute-index]').count(),0);
    for(const width of [1440,1024,768,390,320]) {
      await page.setViewportSize({width,height:1100});
      assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),`No horizontal page overflow at ${width}px`);
      if(width<=1050) {
        await page.locator('.menu').click();
        assert.equal(await page.locator('.menu').getAttribute('aria-expanded'),'true');
        await page.locator('.nav-links a[href="/playground/"]').click();
        assert.equal(await page.locator('.menu').getAttribute('aria-expanded'),'false');
      }
      if(process.env.SCREENSHOT_DIR) {
        fs.mkdirSync(process.env.SCREENSHOT_DIR,{recursive:true});
        await page.screenshot({path:path.join(process.env.SCREENSHOT_DIR,`playground-${width}.png`),fullPage:true});
      }
    }
    assert.deepEqual(errors,[],'No browser exceptions');
    assert.ok(requests.every(r=>r.url.startsWith(origin+'/')&&r.method==='GET'),'Only same-origin static GET requests; no policy/request uploads');
    console.log('PASS browser: top navigation, actual WASM samples and errors, validation, principals, typed attributes, array controls, reset, mobile menus, 320–1440px overflow, browser-only requests');
  } finally {if(browser)await browser.close();await new Promise(resolve=>server.close(resolve));}
})().catch(e=>{console.error(e);process.exitCode=1;});
