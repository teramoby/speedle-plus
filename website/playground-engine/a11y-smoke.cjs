/* Home-page accessibility: axe color-contrast on the English and Chinese home
   pages at desktop and phone widths. Requires Playwright and axe-core in the
   development environment, like site-smoke.cjs.
   The check is hermetic: every request to an origin other than the local server
   is aborted at once and counted, so a slow or unreachable third party (for
   example the analytics beacon) can neither delay nor fail the load event that
   the check waits for. Stylesheets decide contrast, so a stylesheet request that
   is blocked (another origin, whether linked, imported or added by script) or
   does not load fails the gate by URL: serve a local copy instead.
   Run from the repository root: node website/playground-engine/a11y-smoke.cjs
   Optional CHROMIUM_PATH selects a Chrome/Chromium binary. */
const {chromium} = require('playwright');
const axeSource = require('axe-core').source;
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
  const type={'.html':'text/html; charset=utf-8','.css':'text/css','.js':'text/javascript','.wasm':'application/wasm','.svg':'image/svg+xml','.png':'image/png','.ico':'image/x-icon','.txt':'text/plain'}[path.extname(file)] || 'application/octet-stream';
  fs.readFile(file,(error,bytes)=>{if(error){res.writeHead(404).end();return;}res.setHeader('content-type',type);res.end(bytes);});
});
(async()=>{
  await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
  const origin=`http://127.0.0.1:${server.address().port}`;
  const blockedOrigins=new Set();
  let blockedRequests=0;
  let browser;
  try {
    browser=await chromium.launch({headless:true,...(process.env.CHROMIUM_PATH?{executablePath:process.env.CHROMIUM_PATH}:{})});
    let checks=0;
    for (const [route,label] of [['/','English home'],['/zh/','Chinese home']]) {
      for (const width of [1440,390,320]) {
        const page=await browser.newPage({viewport:{width,height:1100}});
        // Stylesheets decide contrast: record every stylesheet request that is blocked or fails,
        // including @import and script-added ones. Chromium gives a failed stylesheet an empty
        // sheet, so the page itself cannot tell.
        // One entry per URL: the first reason wins (an HTTP error, then the aborted body).
        const sheetProblems=new Map();
        page.on('response',response=>{if(response.request().resourceType()==='stylesheet'&&!response.ok()&&!sheetProblems.has(response.url()))sheetProblems.set(response.url(),`HTTP ${response.status()}`);});
        page.on('requestfailed',request=>{
          if (request.resourceType()!=='stylesheet'||sheetProblems.has(request.url())) return;
          let local=false;
          try {local=new URL(request.url()).origin===origin;} catch {}
          sheetProblems.set(request.url(),local?(request.failure()?.errorText||'failed'):`blocked: outside ${origin}, serve a local copy`);
        });
        await page.route('**/*',request=>{
          let url;
          try {url=new URL(request.request().url());} catch {return request.abort();}
          if (url.origin===origin) return request.continue();
          blockedOrigins.add(url.origin);
          blockedRequests++;
          return request.abort();
        });
        // Every other origin is aborted at once, so nothing outside the local server can delay load.
        await page.goto(origin+route,{waitUntil:'load',timeout:30000});
        assert.ok(await page.$('main'),`the ${label} at ${width}px has no main element`);
        const problems=[...sheetProblems].map(([url,reason])=>`${url} (${reason})`);
        assert.deepEqual(problems,[],`the ${label} at ${width}px could not load stylesheets: ${problems.join(', ')}`);
        await page.addScriptTag({content:axeSource});
        const report=await page.evaluate(async()=>{
          const result=await axe.run(document,{runOnly:{type:'rule',values:['color-contrast']}});
          return {
            violations:result.violations.flatMap(rule=>rule.nodes.map(node=>({target:node.target,summary:node.failureSummary}))),
            unchecked:result.incomplete.flatMap(rule=>rule.nodes.map(node=>node.target)),
          };
        });
        assert.deepEqual(report.violations,[],`axe color-contrast violations on the ${label} at ${width}px: ${JSON.stringify(report.violations,null,2)}`);
        checks++;
        console.log(`PASS axe color-contrast: ${label} at ${width}px (${report.unchecked.length} gradient or pseudo-element nodes not machine-checkable)`);
        await page.close();
      }
    }
    console.log(`PASS ${checks} home-page axe color-contrast checks, 0 violations; served only from ${origin}, ${blockedRequests} cross-origin request(s) blocked${blockedOrigins.size?`: ${[...blockedOrigins].join(', ')}`:''}`);
  } finally {if(browser)await browser.close();await new Promise(resolve=>server.close(resolve));}
})().catch(error=>{console.error(error);process.exitCode=1;});
