/* Home-page accessibility: axe color-contrast on the English and Chinese home
   pages at desktop and phone widths. Requires Playwright and axe-core in the
   development environment, like site-smoke.cjs.
   The check is hermetic: every request to an origin other than the local server
   is aborted at once and counted, so a slow or unreachable third party can
   neither delay nor fail the load event that the check waits for. A blocked
   request fails the gate by URL unless it is a script from an origin listed in
   QUIET_ORIGINS (third parties that cannot affect styles, such as the analytics
   beacon); so does any local HTTP error, any local request that fails (a
   cancellation by the page is only reported, and only for types that cannot
   affect rendering, such as pings or media) and any page script error. The
   check waits for the network to settle after load and then until no request
   is in flight and one second has passed with no request starting or ending
   and no DOM change; axe must finish without the page requesting anything.
   WebSockets and web workers, which page.route cannot intercept, fail the
   gate. Serve a local copy of any resource the page needs.
   Run from the repository root: node website/playground-engine/a11y-smoke.cjs
   Optional CHROMIUM_PATH selects a Chrome/Chromium binary. */
const {chromium} = require('playwright');
const axeSource = require('axe-core').source;
const http = require('node:http');
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
const root = path.resolve(__dirname, '../../docs');
// Third-party origins whose scripts the home pages load and that cannot affect styles:
// their script requests are blocked and counted without failing the gate. Any other
// request to them (a stylesheet, fetch, image, ...) fails like any other origin.
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
          // Count DOM changes (elements, attributes, text) so the check can wait until the page is still.
          window.__smokeMutations=0;
          new MutationObserver(records=>{window.__smokeMutations+=records.length;}).observe(document,{subtree:true,childList:true,attributes:true,characterData:true});
        });
        // Record every request problem, one entry per URL (the first reason wins, for example
        // an HTTP error before the aborted body). Chromium gives a stylesheet that failed to
        // load an empty sheet, so the page itself cannot tell.
        const problems=new Map();
        // A request the page cancels itself (net::ERR_ABORTED) is exempt only for types that
        // cannot affect rendering; it is listed in the PASS line rather than ignored. Every
        // other cancellation fails, including fetch/XHR, which may carry CSS.
        const cancelled=[];
        const renderIrrelevant=new Set(['ping','manifest','eventsource','websocket','media','texttrack','cspreport','prefetch']);
        const parse=value=>{try {return new URL(value);} catch {return null;}};
        // Every request, with the time it started, and page script failures.
        let requests=0, pending=0, lastActivityAt=Date.now();
        page.on('request',()=>{requests++;pending++;lastActivityAt=Date.now();});
        const settled=()=>{pending=Math.max(0,pending-1);lastActivityAt=Date.now();};
        page.on('requestfinished',settled);
        page.on('requestfailed',settled);
        // Requests from web workers can bypass page.route, so a page that starts one fails.
        page.on('worker',worker=>problems.set(worker.url(),'web worker: its requests may bypass the route below'));
        page.on('pageerror',error=>{const key=`pageerror ${problems.size}`;problems.set(key,`page script error: ${error.message}`);});
        page.on('response',response=>{
          if (response.status()<400||problems.has(response.url())) return;
          if (parse(response.url())?.origin===origin) problems.set(response.url(),`HTTP ${response.status()}`);
        });
        page.on('requestfailed',request=>{
          const url=parse(request.url());
          if (problems.has(request.url())) return;
          if (url?.origin===origin) {
            const reason=request.failure()?.errorText||'failed';
            if (reason==='net::ERR_ABORTED'&&renderIrrelevant.has(request.resourceType())) cancelled.push(`${request.url()} (${request.resourceType()})`);
            else problems.set(request.url(),`${request.resourceType()}: ${reason}`);
          } else if (!(QUIET_ORIGINS.has(url?.origin)&&request.resourceType()==='script')) {
            problems.set(request.url(),`blocked: outside ${origin}; serve a local copy`);
          }
        });
        const assertNoProblems=when=>{
          const list=[...problems].map(([url,reason])=>`${url} (${reason})`);
          assert.deepEqual(list,[],`the ${label} at ${width}px had problems ${when}: ${list.join(', ')}`);
        };
        // WebSockets do not pass through page.route: refuse every one (it never reaches a server)
        // and fail the gate.
        await page.routeWebSocket(/.*/,ws=>{problems.set(ws.url(),'websocket: not allowed in the a11y smoke');ws.close().catch(()=>{});});
        // Consume every route outcome: a decision still in flight when the page closes must not
        // become an unhandled rejection that hides the real failure.
        await page.route('**/*',async route=>{
          let url;
          try {url=new URL(route.request().url());} catch {await route.abort().catch(()=>{});return;}
          if (url.origin===origin) {
            // A failed continue also fires requestfailed, which classifies it.
            await route.continue().catch(()=>{});
            return;
          }
          blockedOrigins.add(url.origin);
          blockedRequests++;
          await route.abort().catch(()=>{});
        });
        // Every other origin is aborted at once, so nothing outside the local server can delay load.
        await page.goto(origin+route,{waitUntil:'load',timeout:30000});
        try {
          await page.waitForLoadState('networkidle',{timeout:15000});
        } catch (error) {
          throw new Error(`the ${label} at ${width}px kept the network busy for 15s after load`,{cause:error});
        }
        // networkidle only means 500 ms without requests. Also require no request in flight and
        // one second with no request starting or ending and no DOM change, so late styles (a
        // timer or load handler, or a slow response) are in place.
        const settleBy=Date.now()+10000;
        for (let mutations=await page.evaluate(()=>window.__smokeMutations);;) {
          await page.waitForTimeout(250);
          const now=await page.evaluate(()=>window.__smokeMutations);
          if (now!==mutations) {mutations=now;lastActivityAt=Math.max(lastActivityAt,Date.now());}
          if (pending===0&&Date.now()-lastActivityAt>=1000) break;
          assert.ok(Date.now()<settleBy,`the ${label} at ${width}px did not settle (requests or DOM changes) within 10s after load`);
        }
        assert.ok(await page.$('main'),`the ${label} at ${width}px has no main element`);
        assert.equal(await page.evaluate(()=>Boolean(navigator.serviceWorker&&navigator.serviceWorker.controller)),false,`the ${label} at ${width}px is controlled by a service worker`);
        assertNoProblems('before the contrast check');
        assert.equal(pending,0,`the ${label} at ${width}px still has requests in flight before the contrast check`);
        await page.addScriptTag({content:axeSource});
        const requestsBefore=requests;
        const report=await page.evaluate(async()=>{
          const result=await axe.run(document,{runOnly:{type:'rule',values:['color-contrast']}});
          return {
            violations:result.violations.flatMap(rule=>rule.nodes.map(node=>({target:node.target,summary:node.failureSummary}))),
            unchecked:result.incomplete.flatMap(rule=>rule.nodes.map(node=>node.target)),
          };
        });
        // The page must not have loaded anything while axe measured it; with no new request, no
        // request can still fail after the scan, so the problems are complete here.
        assert.equal(requests,requestsBefore,`the ${label} at ${width}px requested resources during the contrast check`);
        assertNoProblems('during the contrast check');
        assert.deepEqual(report.violations,[],`axe color-contrast violations on the ${label} at ${width}px: ${JSON.stringify(report.violations,null,2)}`);
        checks++;
        console.log(`PASS axe color-contrast: ${label} at ${width}px (${report.unchecked.length} gradient or pseudo-element nodes not machine-checkable${cancelled.length?`; ${cancelled.length} local request(s) cancelled by the page: ${cancelled.join(', ')}`:''})`);
        await page.close();
      }
    }
    console.log(`PASS ${checks} home-page axe color-contrast checks, 0 violations; served only from ${origin}, ${blockedRequests} cross-origin request(s) blocked${blockedOrigins.size?`: ${[...blockedOrigins].join(', ')}`:''}`);
  } finally {if(browser)await browser.close();await new Promise(resolve=>server.close(resolve));}
})().catch(error=>{console.error(error);process.exitCode=1;});
