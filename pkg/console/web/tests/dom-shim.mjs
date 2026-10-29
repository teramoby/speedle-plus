/**
 * Just enough DOM for the console's views to run in Node. It checks wiring,
 * not layout or real browser behaviour; Playwright covers those.
 */
function matchOne(el, sel) {
  const tag = sel.match(/^[a-zA-Z][\w-]*/);
  if (tag && el.tagName !== tag[0].toLowerCase()) return false;
  for (const m of sel.matchAll(/#([\w-]+)/g)) if (el.getAttribute('id') !== m[1]) return false;
  for (const m of sel.replace(/\[[^\]]*\]/g, '').matchAll(/\.([\w-]+)/g)) if (!el.className.split(/\s+/).includes(m[1])) return false;
  for (const m of sel.matchAll(/\[([^=\]]+)(?:="([^"]*)")?\]/g)) {
    const v = el.getAttribute(m[1]);
    if (v === null || (m[2] !== undefined && v !== m[2])) return false;
  }
  return true;
}

/** Matches a selector with descendant combinators, such as "tbody a". */
function matches(el, selector) {
  const parts = selector.trim().split(/\s+/);
  if (!matchOne(el, parts.pop())) return false;
  let n = el.parentElement;
  while (parts.length && n) {
    if (matchOne(n, parts[parts.length - 1])) parts.pop();
    n = n.parentElement;
  }
  return parts.length === 0;
}

class Node {
  constructor(tag, doc) {
    this.tagName = tag; this.ownerDocument = doc; this.children = []; this.parentElement = null;
    this.attrs = {}; this.listeners = {}; this._text = ''; this.value = ''; this.checked = false; this.disabled = false;
  }
  setAttribute(k, v) {
    this.attrs[k] = String(v);
    if (k === 'disabled') this.disabled = true;
    if (k === 'hidden') this.hidden = true;
    if (k === 'selected' && this.parentElement) this.parentElement.value = this.value;
  }
  getAttribute(k) { return k in this.attrs ? this.attrs[k] : null; }
  removeAttribute(k) { delete this.attrs[k]; }
  get id() { return this.attrs.id || ''; }
  set id(v) { this.attrs.id = v; }
  get className() { return this.attrs.class || ''; }
  set className(v) { this.attrs.class = v; }
  get classList() {
    const list = () => this.className.split(/\s+/).filter(Boolean);
    return {
      add: (c) => { this.className = [...new Set([...list(), c])].join(' '); },
      remove: (c) => { this.className = list().filter((x) => x !== c).join(' '); },
      contains: (c) => list().includes(c),
    };
  }
  get hidden() { return this._hidden || false; }
  set hidden(v) { this._hidden = Boolean(v); }
  get textContent() { return this._text + this.children.map((c) => c.textContent).join(''); }
  set textContent(v) { this.children = []; this._text = String(v); }
  append(...nodes) {
    for (const n of nodes) {
      const node = typeof n === 'string' ? this.ownerDocument.createTextNode(n) : n;
      node.parentElement?.removeChild(node);
      node.parentElement = this;
      this.children.push(node);
      if (this.tagName === 'select' && node.tagName === 'option' && (this.children.length === 1 || 'selected' in node.attrs)) this.value = node.value;
    }
  }
  after(node) {
    const siblings = this.parentElement.children;
    node.parentElement = this.parentElement;
    siblings.splice(siblings.indexOf(this) + 1, 0, node);
  }
  removeChild(node) { this.children = this.children.filter((c) => c !== node); node.parentElement = null; }
  remove() { this.parentElement?.removeChild(this); }
  replaceChildren(...nodes) { this.children = []; this._text = ''; this.append(...nodes); }
  querySelectorAll(selector) {
    const out = [];
    const walk = (n) => { for (const c of n.children) { if (selector.split(',').some((s) => matches(c, s))) out.push(c); walk(c); } };
    walk(this);
    return out;
  }
  querySelector(selector) { return this.querySelectorAll(selector)[0] || null; }
  addEventListener(type, fn) { (this.listeners[type] ||= []).push(fn); }
  dispatchEvent(event) {
    event.target ||= this;
    event.preventDefault ||= () => { event.defaultPrevented = true; };
    for (let n = this; n; n = n.parentElement) for (const fn of n.listeners[event.type] || []) fn(event);
    return !event.defaultPrevented;
  }
  click() { this.dispatchEvent({ type: 'click' }); if (this.getAttribute('type') === 'submit') this.closestForm()?.dispatchEvent({ type: 'submit' }); }
  closestForm() { for (let n = this; n; n = n.parentElement) if (n.tagName === 'form') return n; return null; }
  focus() { this.ownerDocument.activeElement = this; }
}

export function installDOM() {
  const doc = new Node('#document', null);
  doc.ownerDocument = doc;
  doc.createElement = (tag) => new Node(tag.toLowerCase(), doc);
  doc.createElementNS = (ns, tag) => new Node(tag, doc);
  doc.createTextNode = (text) => { const n = new Node('#text', doc); n._text = String(text); return n; };
  doc.getElementById = (id) => doc.querySelector(`#${id}`);
  doc.documentElement = new Node('html', doc);
  doc.body = new Node('body', doc);
  doc.append(doc.body);
  globalThis.document = doc;
  return doc;
}

/** Waits for pending promises and timers. */
export const settle = () => new Promise((r) => setTimeout(r, 5));

/** Sets an input's value and fires input. */
export function type(el, value) {
  el.value = value;
  el.dispatchEvent({ type: 'input' });
}
