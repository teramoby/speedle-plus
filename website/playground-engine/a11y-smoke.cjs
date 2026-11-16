/* Home-page accessibility: axe color-contrast on the English and Chinese home
   pages at desktop and phone widths. Requires Playwright and axe-core in the
   development environment, like site-smoke.cjs.
   The check is hermetic: every request to an origin other than the local
   server is aborted at once and counted, so a slow or unreachable third party
   can neither delay nor fail the load event that the check waits for. A
   blocked request fails the gate by URL unless it is a script from an origin
   listed in QUIET_ORIGINS (origins that cannot affect styles, such as the
   analytics beacon and the loopback probe); so does any local HTTP error, any
   local request that fails (a cancellation by the page is only reported, and
   only for types that cannot affect rendering, such as pings or media). The
   check waits for the network to settle after load: no request in flight and
   one second with no request starting or ending (a page that keeps loading
   resources fails). It then waits up to three seconds for the DOM and the
   stylesheets to stop changing and for CSS animations and transitions to
   finish: a page that still has one running after that window fails, because
   a few samples cannot cover an animation. A page that keeps changing its
   DOM or its CSSOM is only reported in the PASS line, but never counts as
   settled: axe samples three moments 200 ms apart and a violation in any of
   them fails. Styles are stricter: from before axe is injected until the
   sampling ends, every stylesheet's rule text, media and disabled state and
   every inline style attribute must stay identical, or the gate fails. Open
   shadow roots are included in the DOM observation and stylesheet
   fingerprints, including their style/link elements, inline style attributes,
   adopted sheets and recursively imported sheets.
   Any page script error fails the gate. WebSockets and web workers, which
   page.route cannot intercept, fail the gate. Serve a local copy of any
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
// Origins whose scripts cannot affect styles: the third-party origins the home pages
// load, plus the loopback probe origin added at run time. Their script requests are
// blocked and counted without failing the gate. Any other request to them (a stylesheet,
// fetch, image, ...) fails like any other origin.
const QUIET_ORIGINS = new Set(['https://static.cloudflareinsights.com']);
// A second loopback server, on its own port and therefore its own origin, serves a script
// that only sets a flag. Every page must have its request for it aborted by the route and
// must not run it, which proves that cross-origin blocking really works. If interception is
// ever disabled, the probe loads from this server instead of reaching a real third party.
const probeServer = http.createServer((req,res)=>{
  if(req.url!=='/a11y-smoke-route-probe.js'){res.writeHead(404).end();return;}
  res.setHeader('content-type','text/javascript; charset=utf-8');
  res.end('window.__a11yRouteProbeExecuted=true;');
});
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
  await new Promise(resolve=>probeServer.listen(0,'127.0.0.1',resolve));
  const origin=`http://127.0.0.1:${server.address().port}`;
  // A different port on the loopback interface is a different origin, so the route must
  // abort the probe. Its aborted script is as harmless as the analytics beacon: treat its
  // origin as quiet, or every page would fail on the deliberate probe request.
  const probeOrigin=`http://127.0.0.1:${probeServer.address().port}`;
  const ROUTE_PROBE=`${probeOrigin}/a11y-smoke-route-probe.js`;
  QUIET_ORIGINS.add(probeOrigin);
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
        // package.json pins Playwright 1.65.0, which has every API below. Fail with a readable
        // message rather than a TypeError if an older Playwright is installed anyway.
        assert.equal(typeof page.routeWebSocket,'function',`the a11y smoke needs Playwright 1.48 or newer for page.routeWebSocket; the playground-engine package pins 1.65.0 - reinstall website/playground-engine dependencies`);
        await page.addInitScript(()=>{
          if (window.ServiceWorkerContainer) ServiceWorkerContainer.prototype.register=()=>Promise.reject(new DOMException('service workers are disabled in the a11y smoke','SecurityError'));
          // Count DOM changes (elements, attributes, text) so the check can wait until the page is still.
          // Mutations inside a shadow root do not bubble to an observer on document. Walk open
          // shadow roots recursively, observe each one, and repeat the walk whenever state is read
          // so roots added by page script join the watch. Root discovery also feeds sheets() below.
          window.__smokeMutations=0;
          const observedRoots=new WeakSet();
          const rootIds=new WeakMap();
          let nextRootId=1;
          const observer=new MutationObserver(records=>{window.__smokeMutations+=records.length;});
          const observation={subtree:true,childList:true,attributes:true,characterData:true};
          window.__smokeSyncRoots=()=>{
            const roots=[document], known=new Set(roots);
            for(let index=0;index<roots.length;index++) {
              for(const element of roots[index].querySelectorAll('*')) {
                if(element.shadowRoot&&!known.has(element.shadowRoot)) {
                  known.add(element.shadowRoot);
                  roots.push(element.shadowRoot);
                }
              }
            }
            for(const root of roots) {
              if(!rootIds.has(root)) rootIds.set(root,nextRootId++);
              if(!observedRoots.has(root)) {observer.observe(root,observation);observedRoots.add(root);}
            }
            return roots;
          };
          window.__smokeRootId=root=>rootIds.get(root);
          // Inline style values alone cannot tell "the same value moved to another element"
          // from "nothing changed", so each element gets a stable object ID as well.
          const elementIds=new WeakMap();
          let nextElementId=1;
          window.__smokeElementId=element=>{if(!elementIds.has(element))elementIds.set(element,nextElementId++);return elementIds.get(element);};
          window.__smokeSyncRoots();
        });
        // Record every request problem, one entry per URL (the first reason wins, for example
        // an HTTP error before the aborted body). Chromium gives a stylesheet that failed to
        // load an empty sheet, so the page itself cannot tell.
        const problems=new Map();
        // Per-page proof that the exact positive-control URL reached the route below and that
        // Chromium reported the aborted request, not merely the route callback.
        let routeProbeBlocks=0, routeProbeAborts=0;
        // A request the page cancels itself (net::ERR_ABORTED) is exempt only for types that
        // cannot affect rendering; it is listed in the PASS line rather than ignored. Every
        // other cancellation fails, including fetch/XHR, which may carry CSS. These are the
        // resource types Playwright reports; Chromium reports pings, beacons, prefetches and
        // CSP reports as `other`.
        const cancelled=[];
        const renderIrrelevant=new Set(['media','texttrack','eventsource','websocket','manifest','other']);
        const parse=value=>{try {return new URL(value);} catch {return null;}};
        // Every request, with the time it started, and page script failures.
        let requests=0, pending=0, lastRequestAt=Date.now();
        page.on('request',()=>{requests++;pending++;lastRequestAt=Date.now();});
        const settled=()=>{pending=Math.max(0,pending-1);lastRequestAt=Date.now();};
        page.on('requestfinished',settled);
        page.on('requestfailed',settled);
        // Requests from web workers can bypass page.route, so a page that starts one fails.
        page.on('worker',worker=>problems.set(worker.url(),'web worker: its requests may bypass the route below'));
        // An uncaught page script error can leave only part of the page initialized, so it fails
        // the gate rather than letting axe measure that reduced DOM as though it were complete.
        const pageErrors=[];
        page.on('pageerror',error=>pageErrors.push(error.message));
        page.on('response',response=>{
          if(problems.has(response.url())) return;
          const url=parse(response.url());
          // A response from any other origin means the request escaped page.route. This fails even
          // for a quiet-origin script: quiet requests are tolerated only when the route aborts them.
          if(url&&url.origin!==origin) {problems.set(response.url(),`outside ${origin}: response escaped request interception`);return;}
          if(response.status()<400) return;
          if(url?.origin===origin) problems.set(response.url(),`HTTP ${response.status()}`);
        });
        page.on('requestfailed',request=>{
          const url=parse(request.url());
          // The positive control must fail the way an aborted request does, not just pass
          // through the route callback or load from the probe server.
          if(request.url()===ROUTE_PROBE&&request.failure()?.errorText==='net::ERR_ABORTED') routeProbeAborts++;
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
          assert.deepEqual(pageErrors,[],`the ${label} at ${width}px had page script errors ${when}: ${pageErrors.join(' | ')}`);
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
          // Count the probe only when Chromium acknowledged the abort: a callback that ran
          // but whose abort failed may have let the request through.
          if(url.href===ROUTE_PROBE) {if(await route.abort('aborted').then(()=>true,()=>false)) routeProbeBlocks++;return;}
          await route.abort().catch(()=>{});
        });
        // Every other origin is aborted at once, so nothing outside the local server can delay load.
        await page.goto(origin+route,{waitUntil:'load',timeout:30000});
        // Do not infer that routing works merely because no third-party request was observed.
        // The probe script sets a flag in the page if it ever runs, its abort must be acknowledged
        // by the route, and Chromium must report the request as net::ERR_ABORTED: a loaded probe,
        // an uncounted abort or a non-abort failure all fail the gate. A response from a foreign
        // origin is caught by the response handler as well.
        await page.addScriptTag({url:ROUTE_PROBE}).catch(()=>{});
        assert.equal(await page.evaluate(()=>Boolean(window.__a11yRouteProbeExecuted)),false,`the ${label} at ${width}px ran the route positive control, so page.route did not abort it`);
        assertNoProblems('while checking request interception');
        assert.ok(routeProbeBlocks>0,`the ${label} at ${width}px did not intercept and abort the route positive control; request interception may not be active`);
        assert.ok(routeProbeAborts>0,`the ${label} at ${width}px did not see the route positive control fail with net::ERR_ABORTED; the request may not have been aborted`);
        try {
          await page.waitForLoadState('networkidle',{timeout:15000});
        } catch (error) {
          throw new Error(`the ${label} at ${width}px kept the network busy for 15s after load`,{cause:error});
        }
        // The styles must not change while axe measures the page: in the document and every open
        // shadow root, fingerprint every stylesheet (including adopted ones, style/link elements
        // and recursively imported sheets), every inline style attribute, whether each sheet is
        // disabled, its media and the full text of every rule. Include each root even when it has
        // no sheets, so attaching or removing a shadow root is also a visible change.
        const sheets=()=>page.evaluate(()=>{
          // StyleSheetList and CSSRuleList are array-like (length plus an indexed getter) with
          // no specified iterator: Array.from does not rely on the engine exposing one. Every
          // stylesheet is served from the local origin (anything else is aborted), so a
          // stylesheet whose rules cannot be read makes the fingerprint untrustworthy: fail
          // instead of falling back to a comparison that no longer covers the rule text. The
          // full rule text is compared, not a digest: a hash lets two different rule lists
          // compare equal. The pages have a handful of stylesheets, so the size is no concern.
          return JSON.stringify(window.__smokeSyncRoots().map((root,rootIndex)=>{
            // Position and host name alone cannot distinguish a same-tag replacement whose styles
            // happen to be identical. A stable object ID makes that root replacement visible.
            const kind=root===document?'document':`open shadow root at position ${rootIndex} on ${root.host.nodeName}`;
            const scope=`root ${window.__smokeRootId(root)}: ${kind}`;
            const styles=Array.from(root.querySelectorAll('style'));
            const inline=Array.from(root.querySelectorAll('[style]')).map(element=>[window.__smokeElementId(element),element.getAttribute('style')]);
            const linked=Array.from(root.querySelectorAll('link[rel~="stylesheet" i]')).map(link=>link.sheet).filter(Boolean);
            // Explicitly include style/link element sheets as well as ShadowRoot.styleSheets: the
            // latter is not available in every Chromium version, but CSSOM edits must still count.
            const list=Array.from(new Set(Array.from(root.styleSheets||[]).concat(
              Array.from(root.adoptedStyleSheets||[]),styles.map(style=>style.sheet).filter(Boolean),linked)));
            const visited=new WeakSet();
            const fingerprint=sheet=>{
              const name=sheet.href||(sheet.ownerNode&&sheet.ownerNode.nodeName)||'adopted';
              // @import graphs can contain cycles or reach a sheet already listed at the top level.
              // The first visit contains the complete fingerprint, so a repeated reference is enough.
              if(visited.has(sheet)) return [name,'already fingerprinted'];
              visited.add(sheet);
              let rules;
              try {rules=Array.from(sheet.cssRules);}
              catch (error) {throw new Error(`the a11y smoke cannot read the rules of stylesheet ${name} in ${scope}: ${error.message}`);}
              const imports=rules.filter(rule=>rule.type===CSSRule.IMPORT_RULE).map(rule=>{
                // A missing imported sheet leaves the fingerprint incomplete. Fail closed rather
                // than recording only the unchanged @import statement while its rules are unknown.
                if(!rule.styleSheet) throw new Error(`the a11y smoke cannot read imported stylesheet ${rule.href} in ${scope}`);
                return fingerprint(rule.styleSheet);
              });
              return [name,sheet.disabled,sheet.media?sheet.media.mediaText:'',rules.map(rule=>rule.cssText),imports];
            };
            return [
              scope,
              styles.map(style=>[style.textContent,style.media,style.disabled]),
              inline,
              list.map(fingerprint),
            ];
          }));
        });
        // networkidle only means 500 ms without requests. Require no request in flight and one
        // second with no request starting or ending (bounded to 10 s: a page that keeps loading
        // resources fails). Also wait up to 3 s for a quiet DOM: no DOM change, no running
        // animation and no change to the stylesheet fingerprint, since CSS animations and CSSOM
        // edits change colours without a DOM mutation. The quiet gap must outlast the longest gap
        // between changes seen so far, so a page that changes periodically is not mistaken for a
        // still one. A page that keeps changing is only reported, and the scans below sample it
        // three times.
        const settleStart=Date.now();
        const domState=async()=>({...await page.evaluate(()=>({mutations:window.__smokeMutations,runningAnimations:document.getAnimations().filter(animation=>animation.playState==='running').length})),sheets:await sheets()});
        let state=await domState(), domQuietSince=Date.now(), longestChangeGap=0, domSettled=false;
        for (;;) {
          await page.waitForTimeout(250);
          const now=await domState();
          if (now.mutations!==state.mutations||now.sheets!==state.sheets) {
            const at=Date.now();
            longestChangeGap=Math.max(longestChangeGap,at-domQuietSince);
            domQuietSince=at;
          }
          state=now;
          domSettled=Date.now()-domQuietSince>=Math.max(1000,longestChangeGap+250)&&state.runningAnimations===0;
          const networkQuiet=pending===0&&Date.now()-lastRequestAt>=1000;
          if (networkQuiet&&(domSettled||Date.now()-settleStart>=3000)) break;
          assert.ok(Date.now()-settleStart<10000,`the ${label} at ${width}px kept loading resources for 10s after load`);
        }
        // The settle loop above waits for CSS animations and transitions, but stops after three
        // seconds. Sampling cannot cover an animation, so one that is still running is a failure
        // rather than a reason to take a few more snapshots.
        const runningAnimations=await page.evaluate(()=>document.getAnimations().filter(animation=>animation.playState==='running').length);
        assert.equal(runningAnimations,0,`the ${label} at ${width}px still has CSS animations or transitions running after the 3s settle window`);
        assert.ok(await page.$('main'),`the ${label} at ${width}px has no main element`);
        assert.equal(await page.evaluate(()=>Boolean(navigator.serviceWorker&&navigator.serviceWorker.controller)),false,`the ${label} at ${width}px is controlled by a service worker`);
        assertNoProblems('before the contrast check');
        assert.equal(pending,0,`the ${label} at ${width}px still has requests in flight before the contrast check`);
        // No request may start from here until the scan ends, including while axe is injected.
        const requestsBefore=requests, sheetsBefore=await sheets();
        await page.addScriptTag({content:axeSource});
        assert.equal(pending,0,`the ${label} at ${width}px started a request while axe was injected`);
        // CSS animations and transitions have already finished (an animation seen running is a
        // failure above), so axe measures a still page. When the DOM has not settled, or any scan
        // sees a DOM change or a change to the stylesheet fingerprint, sample three moments 200 ms
        // apart: a violation in any scan fails the gate. A scan that sees an animation start is a
        // failure too, for the same reason as above.
        // Once a change is seen the budget stays at three scans, even if a later scan happens to be
        // still. Mutation counts and the stylesheet fingerprint carry from the end of one scan into
        // the next, so changes during the 200 ms gaps count too.
        let mutationsAt=await page.evaluate(()=>window.__smokeMutations), sheetsAt=sheetsBefore;
        const scan=async()=>{
          const previousSheets=sheetsAt;
          const result=await page.evaluate(async before=>{
            // Discover roots added since the last fingerprint before measuring, so their later
            // mutations are observed even when the new root has no stylesheet of its own.
            window.__smokeSyncRoots();
            const runningAnimations=()=>document.getAnimations().filter(animation=>animation.playState==='running').length;
            const animated=runningAnimations();
            const result=await axe.run(document,{runOnly:{type:'rule',values:['color-contrast']}});
            return {
              violations:result.violations.flatMap(rule=>rule.nodes.map(node=>({target:node.target,summary:node.failureSummary}))),
              unchecked:result.incomplete.flatMap(rule=>rule.nodes.map(node=>node.target)),
              mutations:window.__smokeMutations-before,
              reached:window.__smokeMutations,
              animated:Boolean(animated||runningAnimations()),
            };
          },mutationsAt);
          mutationsAt=result.reached;
          sheetsAt=await sheets();
          return {...result,sheetsChanged:sheetsAt!==previousSheets};
        };
        const scans=[await scan()];
        while (scans.length<3&&(!domSettled||scans.some(result=>result.mutations>0||result.sheetsChanged))) {
          await page.waitForTimeout(200);
          scans.push(await scan());
        }
        // A style change observed between consecutive sampling snapshots fails even if a later
        // snapshot shows it reverted. Fingerprints carry across the 200 ms gaps between scans.
        assert.ok(scans.every(result=>!result.sheetsChanged),`the ${label} at ${width}px changed its styles during the contrast check`);
        // An animation that starts after the settle window would otherwise be measured at one or
        // three instants only; failing is the only honest outcome for the documented invariant.
        assert.ok(scans.every(result=>!result.animated),`the ${label} at ${width}px ran a CSS animation or transition during the contrast check`);
        const uniqueTargets=(items,key,seen)=>items.filter(item=>{const id=JSON.stringify(key(item));if(seen.has(id))return false;seen.add(id);return true;});
        const report={
          violations:uniqueTargets(scans.flatMap(result=>result.violations),violation=>violation.target,new Set()),
          unchecked:uniqueTargets(scans.flatMap(result=>result.unchecked),target=>target,new Set()),
        };
        const scanMutations=scans.reduce((sum,result)=>sum+result.mutations,0);
        // The page must not have loaded anything while axe measured it; with no new request, no
        // request can still fail after the scan, so the problems are complete here.
        assert.equal(requests,requestsBefore,`the ${label} at ${width}px requested resources during the contrast check`);
        assert.equal(pending,0,`the ${label} at ${width}px still has requests in flight after the contrast check`);
        // Compared as a boolean: a failing assert.equal would print both whole fingerprints.
        assert.ok((await sheets())===sheetsBefore,`the ${label} at ${width}px changed its styles during the contrast check`);
        assertNoProblems('during the contrast check');
        assert.deepEqual(report.violations,[],`axe color-contrast violations on the ${label} at ${width}px: ${JSON.stringify(report.violations,null,2)}`);
        checks++;
        console.log(`PASS axe color-contrast: ${label} at ${width}px (${report.unchecked.length} gradient or pseudo-element nodes not machine-checkable${cancelled.length?`; ${cancelled.length} local request(s) cancelled by the page: ${cancelled.join(', ')}`:''}${domSettled?'':'; the DOM or CSSOM was still changing when measured'}${scans.length>1?`; measured ${scans.length} times (${scanMutations} DOM change(s) during the sampling)`:''})`);
        await page.close();
      }
    }
    console.log(`PASS ${checks} home-page axe color-contrast checks, 0 violations; served only from ${origin}, ${blockedRequests} cross-origin request(s) blocked${blockedOrigins.size?`: ${[...blockedOrigins].join(', ')}`:''}`);
  } finally {
    if(browser)await browser.close();
    await new Promise(resolve=>server.close(resolve));
    await new Promise(resolve=>probeServer.close(resolve));
  }
})().catch(error=>{console.error(error);process.exitCode=1;});
