/** Builds DOM nodes. Text is always set as text, never parsed as HTML. */
export function h(tag, props = {}, ...children) {
  const el = document.createElement(tag);
  for (const [key, value] of Object.entries(props || {})) {
    if (value === undefined || value === null || value === false) continue;
    if (key === 'class') el.className = value;
    else if (key === 'text') el.textContent = value;
    else if (key === 'on') for (const [type, fn] of Object.entries(value)) el.addEventListener(type, fn);
    else if (key === 'value') el.value = value;
    else if (key === 'checked') el.checked = Boolean(value);
    else el.setAttribute(key, value === true ? '' : String(value));
  }
  append(el, children);
  return el;
}

function append(el, children) {
  for (const child of children.flat(Infinity)) {
    if (child === undefined || child === null || child === false) continue;
    el.append(typeof child === 'string' || typeof child === 'number' ? document.createTextNode(String(child)) : child);
  }
}

/** Replaces the children of el. */
export function mount(el, ...children) {
  el.replaceChildren();
  append(el, children);
  return el;
}

/** Builds SVG nodes for the role graph. */
export function svg(tag, attrs = {}, ...children) {
  const el = document.createElementNS('http://www.w3.org/2000/svg', tag);
  for (const [key, value] of Object.entries(attrs)) {
    if (key === 'text') el.textContent = value;
    else if (key === 'on') for (const [type, fn] of Object.entries(value)) el.addEventListener(type, fn);
    else if (value !== undefined && value !== null) el.setAttribute(key, String(value));
  }
  append(el, children);
  return el;
}
