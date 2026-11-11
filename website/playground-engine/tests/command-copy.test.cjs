/* Exercise the unchanged production copy handler without starting a browser. */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const {test} = require('node:test');
const script = fs.readFileSync(path.join(__dirname, '../../../docs/js/command-copy.js'), 'utf8');

for (const [lang, label] of [['en', 'Copy prompt'], ['zh-CN', '复制提示词'], ['ja', 'プロンプトをコピー'], ['ko', '프롬프트 복사'], ['es', 'Copiar el prompt']]) {
  test(`copies the entire ${lang} prompt using the existing component`, async () => {
    const element = () => ({
      dataset: {}, children: [], attributes: {},
      classList: {add() {}},
      setAttribute(name, value) { this.attributes[name] = value; },
      append(child) { this.children.push(child); },
      addEventListener(name, handler) { this[name] = handler; },
    });
    const block = element();
    block.querySelector = () => null;
    const code = {textContent: 'First ask me which projects.\n\nWait for my answer.\n', closest: () => block, classList: {contains: () => true}};
    const content = element();
    content.querySelectorAll = selector => {
      assert.equal(selector, 'pre > code.language-bash, pre > code.language-text');
      return [code];
    };
    let copied;
    const context = {
      document: {
        documentElement: {lang},
        querySelector: () => content,
        createElement: element,
      },
      navigator: {clipboard: {async writeText(text) { copied = text; }}},
      setTimeout() {}, clearTimeout() {},
    };
    vm.runInNewContext(script, context);
    assert.equal(block.children.length, 1);
    const button = block.children[0];
    assert.equal(button.attributes['aria-label'], label);
    await button.click();
    assert.equal(copied, code.textContent);
    assert.equal(button.dataset.state, 'copied');
    assert.equal(content.children[0].attributes['aria-live'], 'polite');
    assert.ok(content.children[0].textContent);
  });
}
