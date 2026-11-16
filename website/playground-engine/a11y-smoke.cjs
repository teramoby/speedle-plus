/* Home-page accessibility: axe color-contrast on the English and Chinese home
   pages at desktop and phone widths. Requires Playwright and axe-core in the
   development environment, like site-smoke.cjs.
   The check is hermetic: every request to an origin other than the local server
   is aborted at once and counted, so a slow or unreachable third party can
   neither delay nor fail the load event that the check waits for. A blocked
   request fails the gate by URL unless its origin is listed in QUIET_ORIGINS
   (third parties that cannot affect styles, such as the analytics beacon), and
   a stylesheet request fails it even then; so does a local HTTP error or a
   local stylesheet that does not load. These are checked once the network has
   settled after load and again after axe has run. Serve a local copy of any
   resource the page needs.
   Run from the repository root: node website/playground-engine/a11y-smoke.cjs
   Optional CHROMIUM_PATH selects a Chrome/Chromium binary. */
const {chromium} = require('playwright');
const axeSource = require('axe-core').source;
const http = require('node:http');
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
const root = path.resolve(__dirname, '../../docs');
// Third-party origins the home pages contact that cannot affect styles: requests to
// them are blocked and counted without failing the gate (stylesheets still fail).
const QUIET_ORIGINS = new Set(['https://static.cloudflareinsights.com']);
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
        // No service workers: their requests would not pass through page.route below. The
        // option alone is not enough in every Chromium build, so registration is also refused
        // before any page script runs, and a controlled page fails the gate.
        const page=await browser.newPage({viewport:{width,height:1100},serviceWorkers:'block'});
        await page.addInitScript(()=>{
          if (window.ServiceWorkerContainer) ServiceWorkerContainer.prototype.register=()=>Promise.reject(new DOMException('service workers are disabled in the a11y smoke','SecurityError'));
        });
        // Record every request problem, one entry per URL (the first reason wins, for example
        // an HTTP error before the aborted body). Chromium gives a stylesheet that failed to
        // load an empty sheet, so the page itself cannot tell.
        const problems=new Map();
        const parse=value=>{try {return new URL(value);} catch {return null;}};
        // A stylesheet, or CSS fetched by script (for example fetch() into a <style> element).
        const isCss=request=>request.resourceType()==='stylesheet'||/\.css$/i.test(parse(request.url())?.pathname||'');
        page.on('response',response=>{
          if (response.status()<400||problems.has(response.url())) return;
          if (parse(response.url())?.origin===origin) problems.set(response.url(),`HTTP ${response.status()}`);
        });
        page.on('requestfailed',request=>{
          const url=parse(request.url());
          if (problems.has(request.url())) return;
          if (url?.origin===origin) {
            if (isCss(request)) problems.set(request.url(),request.failure()?.errorText||'failed');
          } else if (isCss(request)||!QUIET_ORIGINS.has(url?.origin)) {
            problems.set(request.url(),`blocked: outside ${origin}; serve a local copy`);
          }
        });
        const assertNoProblems=when=>{
          const list=[...problems].map(([url,reason])=>`${url} (${reason})`);
          assert.deepEqual(list,[],`the ${label} at ${width}px had request problems ${when}: ${list.join(', ')}`);
        };
        await page.route('**/*',request=>{
          let url;
          try {url=new URL(request.request().url());} catch {return request.abort();}
          if (url.origin===origin) return request.continue();
          blockedOrigins.add(url.origin);
          blockedRequests++;
          return request.abort();
        });
        // Every other origin is aborted at once, so nothing outside the local server can delay load,
        // and the network settles quickly afterwards; waiting for that also catches CSS requested
        // after load (a timer or load handler).
        await page.goto(origin+route,{waitUntil:'load',timeout:30000});
        try {
          await page.waitForLoadState('networkidle',{timeout:15000});
        } catch (error) {
          throw new Error(`the ${label} at ${width}px kept the network busy for 15s after load`,{cause:error});
        }
        assert.ok(await page.$('main'),`the ${label} at ${width}px has no main element`);
        assert.equal(await page.evaluate(()=>Boolean(navigator.serviceWorker&&navigator.serviceWorker.controller)),false,`the ${label} at ${width}px is controlled by a service worker`);
        assertNoProblems('before the contrast check');
        await page.addScriptTag({content:axeSource});
        const report=await page.evaluate(async()=>{
          const result=await axe.run(document,{runOnly:{type:'rule',values:['color-contrast']}});
          return {
            violations:result.violations.flatMap(rule=>rule.nodes.map(node=>({target:node.target,summary:node.failureSummary}))),
            unchecked:result.incomplete.flatMap(rule=>rule.nodes.map(node=>node.target)),
          };
        });
        // A request that failed while axe ran may have changed the styles it measured.
        assertNoProblems('during the contrast check');
        assert.deepEqual(report.violations,[],`axe color-contrast violations on the ${label} at ${width}px: ${JSON.stringify(report.violations,null,2)}`);
        checks++;
        console.log(`PASS axe color-contrast: ${label} at ${width}px (${report.unchecked.length} gradient or pseudo-element nodes not machine-checkable)`);
        await page.close();
      }
    }
    console.log(`PASS ${checks} home-page axe color-contrast checks, 0 violations; served only from ${origin}, ${blockedRequests} cross-origin request(s) blocked${blockedOrigins.size?`: ${[...blockedOrigins].join(', ')}`:''}`);
  } finally {if(browser)await browser.close();await new Promise(resolve=>server.close(resolve));}
})().catch(error=>{console.error(error);process.exitCode=1;});
