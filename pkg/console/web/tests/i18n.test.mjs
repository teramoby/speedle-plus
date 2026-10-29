import assert from 'node:assert/strict';
import test from 'node:test';
import { getLocale, initLocale, messages, setLocale, t } from '../static/i18n.js';
import { displayStrings } from './strings.mjs';

const placeholders = (s) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();

test('every display string has a Chinese translation', () => {
  const strings = displayStrings();
  assert.ok(strings.length > 50, `only ${strings.length} strings found`);
  const missing = strings.filter((s) => messages[s] === undefined);
  assert.deepEqual(missing, []);
});

test('translations are used and keep their placeholders', () => {
  const used = new Set(displayStrings());
  assert.deepEqual(Object.keys(messages).filter((k) => !used.has(k)), []);
  for (const [en, zh] of Object.entries(messages)) assert.deepEqual(placeholders(zh), placeholders(en), en);
});

test('t translates and fills placeholders', () => {
  setLocale('en');
  assert.equal(t('Deleted {name}.', { name: 'p1' }), 'Deleted p1.');
  assert.equal(t('Unknown {x}'), 'Unknown {x}');
  setLocale('zh');
  assert.equal(getLocale(), 'zh');
  assert.equal(t('Deleted {name}.', { name: 'p1' }), '已删除 p1。');
  assert.equal(t('not translated'), 'not translated');
  setLocale('fr');
  assert.equal(getLocale(), 'en');
});

test('initLocale prefers the saved choice, then the browser', () => {
  const store = {};
  globalThis.localStorage = { getItem: (k) => store[k] ?? null, setItem: (k, v) => { store[k] = v; } };
  Object.defineProperty(globalThis, 'navigator', { value: { language: 'zh-CN' }, configurable: true });
  assert.equal(initLocale(), 'zh');
  setLocale('en');
  assert.equal(initLocale(), 'en');
  globalThis.localStorage = { getItem: () => { throw new Error('blocked'); }, setItem: () => { throw new Error('blocked'); } };
  assert.equal(initLocale(), 'zh');
  setLocale('en');
  delete globalThis.localStorage;
});
