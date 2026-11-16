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
   only for types that cannot affect rendering, such as pings or media).
   axe measures one moment, so the check measures only a still page. After
   load it waits until the network is quiet (no request in flight and one
   second with no request starting or ending; a page that keeps loading
   resources fails after 10 s) and the page is still: no DOM change of any
   kind (elements, attributes including class and style, text), no change to
   the stylesheet fingerprint and no running CSS animation or transition, for
   longer than one second and longer than the longest gap between changes
   seen so far. A page that keeps changing fails after 5 s. From before axe is
   injected until its single scan ends, any request, DOM change, style change,
   animation or transition start, or running animation fails the gate; axe
   goes in as the smoke's own tagged script element, which is not counted.
   Running animations are counted in the document and every open shadow root.
   The proof covers the top document and its shadow roots only, so a page
   with a frame fails until the smoke covers frames. Each page runs in its own
   browser context, whose route aborts every cross-origin request. Before the
   page loads, a blank probe page in that context proves the route works by
   loading a script from another loopback origin, so the measured page's own
   Content Security Policy cannot block or weaken the proof. The stylesheet
   fingerprint covers
   every stylesheet's rule text, media and disabled state and every inline
   style attribute. Open shadow roots are included in the DOM observation and
   the fingerprint, with their style/link elements, inline style attributes,
   adopted sheets and recursively imported sheets.
   Any page script error fails the gate. WebSockets and web workers, which
   the route cannot intercept, fail the gate. Serve a local copy of any
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
// The route aborts every cross-origin request with this distinctive error, so a failure the route
// caused cannot be confused with a request the page cancelled itself (net::ERR_ABORTED).
// Chromium reports it as net::ERR_BLOCKED_BY_CLIENT with a suffix naming the blocker (.Inspector).
const ROUTE_ABORT_ERROR='net::ERR_BLOCKED_BY_CLIENT';
const routeAborted=errorText=>typeof errorText==='string'&&errorText.startsWith(ROUTE_ABORT_ERROR);
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
// A listen failure is an 'error' event, not a callback: reject on it so the run fails instead of
// waiting for a callback that never comes.
const listen=listener=>new Promise((resolve,reject)=>{
  listener.once('error',reject);
  listener.listen(0,'127.0.0.1',()=>{listener.off('error',reject);resolve();});
});
(async()=>{
  let browser;
  try {
    // Both servers start inside the try, so the finally below closes whichever one did start.
    await listen(server);
    await listen(probeServer);
    const origin=`http://127.0.0.1:${server.address().port}`;
    // A different port on the loopback interface is a different origin, so the route must
    // abort the probe. Only the blank probe page requests it, never a measured page.
    const probeOrigin=`http://127.0.0.1:${probeServer.address().port}`;
    const ROUTE_PROBE=`${probeOrigin}/a11y-smoke-route-probe.js`;
    const blockedOrigins=new Set();
    let blockedRequests=0;
    browser=await chromium.launch({headless:true,...(process.env.CHROMIUM_PATH?{executablePath:process.env.CHROMIUM_PATH}:{})});
    let checks=0;
    for (const [route,label] of [['/','English home'],['/zh/','Chinese home']]) {
      for (const width of [1440,390,320]) {
        // Each page gets its own browser context, whose route below covers every page in it. No
        // service workers: their requests would not pass through the route. The option alone is
        // not enough in every Chromium build, so registration is also refused before any page
        // script runs, and a controlled page fails the gate.
        const context=await browser.newContext({viewport:{width,height:1100},serviceWorkers:'block'});
        const page=await context.newPage();
        // package.json pins Playwright 1.65.0, which has every API below. Fail with a readable
        // message rather than a TypeError if an older Playwright is installed anyway.
        assert.equal(typeof context.routeWebSocket,'function',`the a11y smoke needs Playwright 1.48 or newer for context.routeWebSocket; the playground-engine package pins 1.65.0 - reinstall website/playground-engine dependencies`);
        await context.addInitScript(()=>{
          if (window.ServiceWorkerContainer) ServiceWorkerContainer.prototype.register=()=>Promise.reject(new DOMException('service workers are disabled in the a11y smoke','SecurityError'));
          // Count DOM changes (elements, attributes, text) so the check can wait until the page is still.
          // Mutations inside a shadow root do not bubble to an observer on document. Walk open
          // shadow roots recursively, observe each one, and repeat the walk whenever state is read
          // so roots added by page script join the watch. Root discovery also feeds sheets() below.
          window.__smokeMutations=0;
          window.__smokeAnimationStarts=0;
          // When the page last changed, by the page's own clock (the same machine clock as the
          // smoke), so the settle wait can date a change by when it happened, not when a poll saw it.
          window.__smokeLastChangeAt=0;
          const observedRoots=new WeakSet();
          const rootIds=new WeakMap();
          let nextRootId=1;
          // The smoke's own axe script element (data-a11y-smoke="axe") is not a page change.
          const ownElement=node=>node.nodeType===1&&node.getAttribute('data-a11y-smoke')==='axe';
          const observer=new MutationObserver(records=>{
            for (const record of records) {
              if(record.type==='childList'&&record.removedNodes.length===0&&record.addedNodes.length>0&&Array.from(record.addedNodes).every(ownElement)) continue;
              window.__smokeMutations++;
              window.__smokeLastChangeAt=Date.now();
            }
          });
          // An animation or transition can start and finish without any DOM or stylesheet change
          // (Element.animate from a timer, for example), so every start counts as a change.
          const countStart=()=>{window.__smokeAnimationStarts++;window.__smokeLastChangeAt=Date.now();};
          const animate=Element.prototype.animate;
          Element.prototype.animate=function(...args){countStart();return animate.apply(this,args);};
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
              if(!observedRoots.has(root)) {
                observer.observe(root,observation);
                // CSS animation and transition events are not composed, so listen on every root.
                for (const type of ['animationstart','transitionrun']) root.addEventListener(type,countStart,true);
                observedRoots.add(root);
              }
            }
            return roots;
          };
          window.__smokeRootId=root=>rootIds.get(root);
          // Inline style values alone cannot tell "the same value moved to another element"
          // from "nothing changed", so each element gets a stable object ID as well.
          const elementIds=new WeakMap();
          let nextElementId=1;
          window.__smokeElementId=element=>{if(!elementIds.has(element))elementIds.set(element,nextElementId++);return elementIds.get(element);};
          // Running animations in the document and in every open shadow root: each root's own
          // getAnimations() reaches the animations inside it.
          window.__smokeRunningAnimations=()=>{
            const running=new Set();
            for (const root of window.__smokeSyncRoots()) for (const animation of root.getAnimations()) if (animation.playState==='running') running.add(animation);
            return running.size;
          };
          window.__smokeSyncRoots();
        });
        // Record every request problem, one entry per URL (the first reason wins, for example
        // an HTTP error before the aborted body). Chromium gives a stylesheet that failed to
        // load an empty sheet, so the page itself cannot tell.
        const problems=new Map();
        // Per-context proof that the exact positive-control URL reached the route below and that
        // Chromium reported the aborted request, not merely the route callback.
        let routeProbeBlocks=0, routeProbeAborts=0;
        // A request the page cancels itself (net::ERR_ABORTED) is exempt only for types that
        // cannot affect rendering; it is listed in the PASS line rather than ignored. Every
        // other cancellation fails, including fetch/XHR, which may carry CSS. These are the
        // resource types Playwright reports; Chromium reports pings, beacons, prefetches and
        // CSP reports as `other`.
        const cancelled=[];
        // Cross-origin requests that the page's own Content Security Policy blocked in the browser:
        // they never left it, exactly as in production, so they are only listed in the PASS line.
        const cspBlocked=[];
        const renderIrrelevant=new Set(['media','texttrack','eventsource','websocket','manifest','other']);
        const parse=value=>{try {return new URL(value);} catch {return null;}};
        // Every request, with the time it started, and page script failures.
        let requests=0, pending=0, lastRequestAt=Date.now();
        page.on('request',()=>{requests++;pending++;lastRequestAt=Date.now();});
        const settled=()=>{pending=Math.max(0,pending-1);lastRequestAt=Date.now();};
        page.on('requestfinished',settled);
        page.on('requestfailed',settled);
        // Requests from web workers can bypass the route, so a page that starts one fails.
        page.on('worker',worker=>problems.set(worker.url(),'web worker: its requests may bypass the route below'));
        // An uncaught page script error can leave only part of the page initialized, so it fails
        // the gate rather than letting axe measure that reduced DOM as though it were complete.
        const pageErrors=[];
        page.on('pageerror',error=>pageErrors.push(error.message));
        page.on('response',response=>{
          if(problems.has(response.url())) return;
          const url=parse(response.url());
          // A response from any other origin means the request escaped the route. This fails even
          // for a quiet-origin script: quiet requests are tolerated only when the route aborts them.
          if(url&&url.origin!==origin) {problems.set(response.url(),`outside ${origin}: response escaped request interception`);return;}
          if(response.status()<400) return;
          if(url?.origin===origin) problems.set(response.url(),`HTTP ${response.status()}`);
        });
        page.on('requestfailed',request=>{
          const url=parse(request.url());
          if (problems.has(request.url())) return;
          if (url?.origin===origin) {
            const reason=request.failure()?.errorText||'failed';
            if (reason==='net::ERR_ABORTED'&&renderIrrelevant.has(request.resourceType())) cancelled.push(`${request.url()} (${request.resourceType()})`);
            else problems.set(request.url(),`${request.resourceType()}: ${reason}`);
          } else {
            // A quiet-origin script is tolerated only when it failed the way the route aborts it.
            // Any other failure (DNS, TLS, connection) means the route never intercepted it.
            const reason=request.failure()?.errorText||'failed';
            if (reason==='csp') {cspBlocked.push(request.url());return;}
            const aborted=routeAborted(reason);
            if (!(aborted&&QUIET_ORIGINS.has(url?.origin)&&request.resourceType()==='script')) {
              problems.set(request.url(),aborted?`blocked: outside ${origin}; serve a local copy`:`outside ${origin} and not aborted by the route (${reason}): request interception was bypassed`);
            }
          }
        });
        const assertNoProblems=when=>{
          const list=[...problems].map(([url,reason])=>`${url} (${reason})`);
          assert.deepEqual(list,[],`the ${label} at ${width}px had problems ${when}: ${list.join(', ')}`);
          assert.deepEqual(pageErrors,[],`the ${label} at ${width}px had page script errors ${when}: ${pageErrors.join(' | ')}`);
        };
        // WebSockets do not pass through the route: refuse every one (it never reaches a server)
        // and fail the gate.
        await context.routeWebSocket(/.*/,ws=>{problems.set(ws.url(),'websocket: not allowed in the a11y smoke');ws.close().catch(()=>{});});
        // Consume every route outcome: a decision still in flight when the page closes must not
        // become an unhandled rejection that hides the real failure.
        await context.route('**/*',async route=>{
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
          if(url.href===ROUTE_PROBE) {if(await route.abort('blockedbyclient').then(()=>true,()=>false)) routeProbeBlocks++;return;}
          await route.abort('blockedbyclient').catch(()=>{});
        });
        // Do not infer that routing works merely because no third-party request was observed.
        // Before the page loads, a blank probe page in the same context, which shares the route,
        // has no Content Security Policy and runs no site script, loads a script from another
        // loopback origin. The script sets a flag if it ever runs, the route must acknowledge its
        // abort, and Chromium must report the request with the route's own abort error: a loaded
        // probe, an uncounted abort or any other failure fails the gate. The measured page's own
        // CSP therefore cannot block the proof or call for weakening that policy. Event delivery
        // is independent of the script element settling, so wait for both counters, bounded.
        const probe=await context.newPage();
        probe.on('requestfailed',request=>{if(request.url()===ROUTE_PROBE&&routeAborted(request.failure()?.errorText)) routeProbeAborts++;});
        await probe.goto('about:blank');
        await probe.addScriptTag({url:ROUTE_PROBE}).catch(()=>{});
        const probeDeadline=Date.now()+5000;
        while ((routeProbeBlocks===0||routeProbeAborts===0)&&Date.now()<probeDeadline) await new Promise(resolve=>setTimeout(resolve,25));
        assert.equal(await probe.evaluate(()=>Boolean(window.__a11yRouteProbeExecuted)),false,`the route positive control ran for the ${label} at ${width}px, so the context route did not abort it`);
        assert.ok(routeProbeBlocks>0,`the context route for the ${label} at ${width}px did not intercept and abort the route positive control; request interception may not be active`);
        assert.ok(routeProbeAborts>0,`the route positive control for the ${label} at ${width}px did not fail with ${ROUTE_ABORT_ERROR}; the request may not have been aborted`);
        await probe.close();
        // Every other origin is aborted at once, so nothing outside the local server can delay load.
        await page.goto(origin+route,{waitUntil:'load',timeout:30000});
        assertNoProblems('while loading');
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
        // axe measures one moment, so the smoke measures only a still page. Class or attribute
        // churn, CSSOM edits and CSS animations all change colours without a request, and a few
        // samples cannot cover them. Wait until the network is quiet (no request in flight and one
        // second with no request starting or ending) and the page is still: no DOM change of any
        // kind (elements, attributes including class and style, text), no change to the stylesheet
        // fingerprint and no running animation or transition. The still period must last at least
        // one second and longer than the longest gap between changes seen so far, so a page that
        // changes periodically never counts as still. Both waits are bounded and fail: a page that
        // still starts or finishes a request 10 s after load fails, and so does a page that still
        // changes 5 s after load. A page that stops inside those bounds gets its full quiet period.
        const pageState=async()=>({...await page.evaluate(()=>({mutations:window.__smokeMutations,animationStarts:window.__smokeAnimationStarts,lastChangeAt:window.__smokeLastChangeAt,runningAnimations:window.__smokeRunningAnimations()})),sheets:await sheets()});
        const settleStart=Date.now();
        let state=await pageState(), stillSince=Date.now(), longestChangeGap=0;
        for (;;) {
          await page.waitForTimeout(250);
          const now=await pageState();
          if (now.mutations!==state.mutations||now.sheets!==state.sheets||now.animationStarts!==state.animationStarts||now.runningAnimations>0) {
            // Date a DOM change or an animation start by when the page saw it, not when this poll
            // noticed it. A stylesheet-only edit or a still-running animation has no page timestamp
            // and is dated by this poll.
            const counted=now.mutations!==state.mutations||now.animationStarts!==state.animationStarts;
            const dated=counted&&now.sheets===state.sheets&&now.runningAnimations===0;
            const at=dated?Math.max(stillSince,Math.min(Date.now(),now.lastChangeAt)):Date.now();
            longestChangeGap=Math.max(longestChangeGap,at-stillSince);
            stillSince=at;
          }
          state=now;
          const networkQuiet=pending===0&&Date.now()-lastRequestAt>=1000;
          const still=Date.now()-stillSince>=Math.max(1000,longestChangeGap+250);
          if (networkQuiet&&still) break;
          // The budgets bound when the last change may happen, not when stillness must be proven:
          // a page that stops changing inside its budget always gets the full quiet period above.
          const elapsed=Date.now()-settleStart;
          assert.ok(lastRequestAt-settleStart<10000&&(pending===0||elapsed<10000),`the ${label} at ${width}px kept loading resources for 10s after load`);
          assert.ok(stillSince-settleStart<5000,`the ${label} at ${width}px kept changing its DOM, styles or animations for 5s after load; the contrast check measures a still page only`);
        }
        assert.ok(await page.$('main'),`the ${label} at ${width}px has no main element`);
        // axe would also measure same-origin frames, but the still-page proof covers only the top
        // document and its shadow roots. The home pages have no frames: require that, so a page
        // that adds one fails here until the smoke covers frames. A frame added later is a DOM
        // change and fails as well.
        assert.equal(page.frames().length,1,`the ${label} at ${width}px has ${page.frames().length-1} frame(s); the a11y smoke measures the top document only, so cover frames before adding them`);
        assert.equal(await page.evaluate(()=>Boolean(navigator.serviceWorker&&navigator.serviceWorker.controller)),false,`the ${label} at ${width}px is controlled by a service worker`);
        assertNoProblems('before the contrast check');
        assert.equal(pending,0,`the ${label} at ${width}px still has requests in flight before the contrast check`);
        // From here until the scan ends nothing may change: no request, no DOM change, no style
        // change, no animation start and no running animation. One scan of a still page is the
        // whole measurement. axe goes in as the smoke's own tagged script element, which the DOM
        // observer ignores, so any DOM change while it is injected fails as well.
        const requestsBefore=requests, before=await pageState();
        assert.equal(before.runningAnimations,0,`the ${label} at ${width}px ran a CSS animation or transition before the contrast check`);
        await page.evaluate(source=>{
          const script=document.createElement('script');
          script.setAttribute('data-a11y-smoke','axe');
          script.textContent=source;
          document.head.append(script);
        },axeSource);
        assert.equal(pending,0,`the ${label} at ${width}px started a request while axe was injected`);
        assert.equal(await page.evaluate(()=>window.__smokeMutations),before.mutations,`the ${label} at ${width}px changed its DOM while axe was injected`);
        const result=await page.evaluate(async()=>{
          // Discover shadow roots attached since the last fingerprint, so a change inside one
          // during the scan is observed as well.
          window.__smokeSyncRoots();
          const runningAtStart=window.__smokeRunningAnimations();
          const result=await axe.run(document,{runOnly:{type:'rule',values:['color-contrast']}});
          return {
            runningAtStart,
            violations:result.violations.flatMap(rule=>rule.nodes.map(node=>({target:node.target,summary:node.failureSummary}))),
            unchecked:result.incomplete.flatMap(rule=>rule.nodes.map(node=>node.target)),
          };
        });
        const after=await pageState();
        // The page must not have loaded anything while axe measured it; with no new request, no
        // request can still fail after the scan, so the problems are complete here.
        assert.equal(requests,requestsBefore,`the ${label} at ${width}px requested resources during the contrast check`);
        assert.equal(pending,0,`the ${label} at ${width}px still has requests in flight after the contrast check`);
        assert.equal(after.mutations,before.mutations,`the ${label} at ${width}px changed its DOM during the contrast check`);
        // Compared as a boolean: a failing assert.equal would print both whole fingerprints.
        assert.ok(after.sheets===before.sheets,`the ${label} at ${width}px changed its styles during the contrast check`);
        // An animation that starts and ends inside the scan leaves nothing running afterwards, so
        // check the start count as well as the running animations at both ends of the scan.
        assert.equal(result.runningAtStart,0,`the ${label} at ${width}px ran a CSS animation or transition when the contrast check started`);
        assert.equal(after.animationStarts,before.animationStarts,`the ${label} at ${width}px started a CSS animation or transition during the contrast check`);
        assert.equal(after.runningAnimations,0,`the ${label} at ${width}px ran a CSS animation or transition during the contrast check`);
        assertNoProblems('during the contrast check');
        assert.deepEqual(result.violations,[],`axe color-contrast violations on the ${label} at ${width}px: ${JSON.stringify(result.violations,null,2)}`);
        checks++;
        console.log(`PASS axe color-contrast: ${label} at ${width}px (${result.unchecked.length} gradient or pseudo-element nodes not machine-checkable${cancelled.length?`; ${cancelled.length} local request(s) cancelled by the page: ${cancelled.join(', ')}`:''}${cspBlocked.length?`; ${cspBlocked.length} cross-origin request(s) blocked by the page's own CSP: ${cspBlocked.join(', ')}`:''})`);
        await context.close();
      }
    }
    console.log(`PASS ${checks} home-page axe color-contrast checks, 0 violations; served only from ${origin}, ${blockedRequests} cross-origin request(s) blocked${blockedOrigins.size?`: ${[...blockedOrigins].join(', ')}`:''}`);
  } finally {
    // Cleanup must not replace the check's own failure: a failed browser close is reported and
    // fails the run, but the earlier error still propagates. Both servers shut down either way;
    // drop any connection the browser left open first, or server.close would wait for it.
    if(browser) await browser.close().catch(error=>{console.error(`closing the browser failed: ${error.message}`);process.exitCode=1;});
    for (const listener of [server,probeServer]) listener.closeAllConnections();
    await Promise.all([server,probeServer].map(listener=>new Promise(resolve=>listener.close(()=>resolve()))));
  }
})().catch(error=>{console.error(error);process.exitCode=1;});
