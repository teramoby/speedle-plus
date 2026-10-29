/** Finds the display strings the console passes through t(). */
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

const PATTERNS = [
  /\bt\(\s*'((?:[^'\\]|\\.)+)'/g, // t('…')
  /\bN_\(\s*'((?:[^'\\]|\\.)+)'/g, // N_('…')
  /\bbutton\(\s*'((?:[^'\\]|\\.)+)'/g, // button('…')
  /\bfield\(\s*[^,]+,\s*'((?:[^'\\]|\\.)+)'/g, // field(id, '…')
  /\bhint:\s*'((?:[^'\\]|\\.)+)'/g, // { hint: '…' }
  /\blabel:\s*'((?:[^'\\]|\\.)+)'/g, // { label: '…' }
  /\berror\([^,]+,\s*'((?:[^'\\]|\\.)+)'/g, // error(path, '…')
  /\bmessage:\s*'((?:[^'\\]|\\.)+)'/g, // { message: '…' }
  /^\s*\['([^']+)',/gm, // table columns ['Label', fn]
  /\bt\(\s*role \? '([^']+)' : '([^']+)'/g, // t(role ? '…' : '…')
  /\bt\([^)]*\? '([^']+)' : '([^']+)'\)/g, // t(x ? '…' : '…')
];

export const staticDir = new URL('../static/', import.meta.url).pathname;

export function sourceFiles(dir = staticDir) {
  return readdirSync(dir, { withFileTypes: true }).flatMap((e) => {
    const path = join(dir, e.name);
    if (e.isDirectory()) return sourceFiles(path);
    return e.name.endsWith('.js') && e.name !== 'i18n.js' && !e.name.startsWith('messages') ? [path] : [];
  });
}

export function displayStrings() {
  const found = new Set();
  for (const file of sourceFiles()) {
    const text = readFileSync(file, 'utf8');
    for (const pattern of PATTERNS) {
      for (const m of text.matchAll(pattern)) for (const s of m.slice(1)) if (s) found.add(s.replace(/\\'/g, "'"));
    }
  }
  return [...found].sort();
}
