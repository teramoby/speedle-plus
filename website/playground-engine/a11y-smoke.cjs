/* Home-page accessibility: axe color-contrast on the English and Chinese home
   pages at desktop and phone widths. Requires Playwright and axe-core in the
   development environment, like site-smoke.cjs.
   The check is hermetic: every request to an origin other than the local server
   is aborted, and a page is ready at DOMContentLoaded once its main content and
   local stylesheets are loaded, never at the full load event. A slow or
   unreachable third party (for example the analytics beacon) therefore cannot
   fail or delay the gate.
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
  let browser;
  try {
    browser=await chromium.launch({headless:true,...(process.env.CHROMIUM_PATH?{executablePath:process.env.CHROMIUM_PATH}:{})});
    let checks=0;
    for (const [route,label] of [['/','English home'],['/zh/','Chinese home']]) {
      for (const width of [1440,390,320]) {
        const page=await browser.newPage({viewport:{width,height:1100}});
        await page.route('**/*',request=>{
          let url;
          try {url=new URL(request.request().url());} catch {return request.abort();}
          if (url.origin===origin) return request.continue();
          blockedOrigins.add(url.origin);
          return request.abort();
        });
        await page.goto(origin+route,{waitUntil:'domcontentloaded'});
        await page.waitForFunction(()=>{
          if (!document.body||!document.body.querySelector('main')) return false;
          return [...document.querySelectorAll('link[rel~="stylesheet"]')].every(link=>{
            try {return Boolean(link.sheet&&link.sheet.cssRules);} catch {return false;}
          });
        });
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
    console.log(`PASS ${checks} home-page axe color-contrast checks, 0 violations, 0 requests outside ${origin}${blockedOrigins.size?` (blocked ${[...blockedOrigins].join(', ')})`:''}`);
  } finally {if(browser)await browser.close();await new Promise(resolve=>server.close(resolve));}
})().catch(error=>{console.error(error);process.exitCode=1;});
