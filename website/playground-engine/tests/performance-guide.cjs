/* Shared checks for published pages and fresh Hugo output (issue #346). */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const root = path.resolve(__dirname, '../../..');

module.exports = function checkPerformanceGuides(output) {
  const locales = [
    ['', 'First ask me which open-source authorization projects', 'Wait for my answer before doing anything else.'],
    ['zh', '首先问我', '等待我的回答，再做任何其他事情。'],
    ['ja', '最初に', '私が回答するまで、ほかの作業を始めないでください。'],
    ['ko', '먼저', '제가 답할 때까지 다른 작업을 하지 마세요.'],
    ['es', 'Primero pregúnteme', 'Espere mi respuesta antes de hacer cualquier otra cosa.'],
  ];
  for (const [locale, first, wait] of locales) {
    const source = fs.readFileSync(path.join(root, `docs/hugo/content/docs/performance-testing${locale ? '.' + locale : ''}.md`), 'utf8');
    const body = source.split('+++')[2].trim();
    const prompts = [...body.matchAll(/```text\n([\s\S]*?)\n```/g)];
    assert.equal(prompts.length, 1, `One prompt: ${locale}`);
    assert.equal((body.match(/```/g) || []).length, 2, `No other code blocks: ${locale}`);
    const prompt = prompts[0][1];
    assert.ok(prompt.startsWith(first), `First action asks for projects: ${locale}`);
    assert.ok(prompt.split('\n\n')[0].includes(wait), `Wait before any work: ${locale}`);
    for (const term of ['Speedle', '10', '100', '1,000', '30', 'RBAC', 'ABAC', 'API', 'p50/p95/p99']) {
      assert.ok(prompt.includes(term), `Prompt includes ${term}: ${locale}`);
    }
    assert.ok(body.length < 2600, `Short guide: ${locale}`);
    assert.equal(body.replace(prompts[0][0], '').trim().split(/\n\s*\n/).length, 2, `Only intro and usage outside prompt: ${locale}`);
    assert.doesNotMatch(source, /Open Policy Agent|\bOPA\b|\bRego\b|\bCasbin\b|\bCedar\b|Claude|ChatGPT|DeepSeek/i, `No named alternatives: ${locale}`);
    const prefix = locale ? `${locale}/` : '';
    const html = fs.readFileSync(path.join(output, `${prefix}docs/performance-testing/index.html`), 'utf8');
    const sidebar = html.match(/<aside class="docs-sidebar"[\s\S]*?<\/aside>/)?.[0];
    assert.ok(sidebar, `Documentation sidebar: ${locale}`);
    const misc = {'': 'Misc', zh: '其他', es: 'Otros', ja: 'その他', ko: '기타'}[locale];
    const finalGroup = sidebar.match(/<li data-docs-depth="0" class="docs-nav-section[^>]*><span class="docs-nav-heading">([^<]+)<\/span>\s*<ul[^>]*>\s*<li data-docs-depth="1"[^>]*><a href="([^"]+)"[^>]*>([^<]+)<\/a><\/li>\s*<\/ul>\s*<\/li>\s*<\/ul>\s*<\/li>\s*<\/ul>\s*<\/nav>/);
    assert.ok(finalGroup, `Final sidebar section contains only the nested guide: ${locale}`);
    assert.equal(finalGroup[1], misc, `Localized Misc label: ${locale}`);
    assert.equal(finalGroup[2], `/${prefix}docs/performance-testing/`, `Original guide URL retained: ${locale}`);
    assert.equal((sidebar.match(/href="[^"]*docs\/performance-testing\/"/g) || []).length, 1, `Guide appears once: ${locale}`);
    const code = html.match(/<pre[^>]*><code class="language-text"[^>]*>([\s\S]*?)<\/code><\/pre>/);
    assert.ok(code, `Prompt uses the existing copy selector: ${locale}`);
    const decoded = code[1].replace(/&quot;/g, '"').replace(/&#39;|&#x27;/g, "'").replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&amp;/g, '&');
    assert.equal(decoded.trim(), prompt, `Rendered prompt matches source: ${locale}`);
    assert.equal((html.match(/class="language-text"/g) || []).length, 1, `One rendered prompt: ${locale}`);
    assert.ok(html.includes('id="kube-component"') && html.includes('/js/command-copy.js') && html.includes('/css/command-copy.css'), `Copy component loaded: ${locale}`);
    assert.ok(fs.readFileSync(path.join(output, `${prefix}index.html`), 'utf8').includes(`href="/${prefix}docs/performance-testing/"`), `Homepage link works: ${locale}`);
  }
};
