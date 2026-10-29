/**
 * Lays out a directed graph in layers, left to right: each node goes one
 * layer after its furthest predecessor (cycles are broken first), and nodes
 * within a layer are ordered by the mean position of their neighbours to
 * reduce crossings. The result depends only on the input.
 */
export function layout(nodes, edges, { nodeWidth = 180, nodeHeight = 34, gapX = 70, gapY = 16, sweeps = 4, minLayer = () => 0 } = {}) {
  const ids = nodes.map((n) => n.id).sort();
  const known = new Set(ids);
  const out = new Map(ids.map((id) => [id, []]));
  for (const e of edges) if (known.has(e.from) && known.has(e.to) && e.from !== e.to) out.get(e.from).push(e.to);
  for (const list of out.values()) list.sort();

  // Drop the edges that close a cycle, found by depth-first search that
  // starts from the nodes nothing points to, so a cycle is cut where it is
  // entered rather than at its alphabetically first node.
  const hasIn = new Set();
  for (const list of out.values()) for (const to of list) hasIn.add(to);
  const state = new Map();
  const forward = new Map(ids.map((id) => [id, []]));
  const visit = (id) => {
    state.set(id, 'open');
    for (const next of out.get(id)) {
      if (state.get(next) === 'open') continue;
      forward.get(id).push(next);
      if (!state.has(next)) visit(next);
    }
    state.set(id, 'done');
  };
  for (const id of [...ids.filter((id) => !hasIn.has(id)), ...ids]) if (!state.has(id)) visit(id);

  // Longest-path layering over the remaining acyclic edges.
  const indegree = new Map(ids.map((id) => [id, 0]));
  for (const list of forward.values()) for (const to of list) indegree.set(to, indegree.get(to) + 1);
  const byID = new Map(nodes.map((n) => [n.id, n]));
  const layerOf = new Map(ids.map((id) => [id, minLayer(byID.get(id))]));
  const queue = ids.filter((id) => indegree.get(id) === 0);
  while (queue.length) {
    const id = queue.shift();
    for (const to of forward.get(id)) {
      layerOf.set(to, Math.max(layerOf.get(to), layerOf.get(id) + 1));
      indegree.set(to, indegree.get(to) - 1);
      if (indegree.get(to) === 0) queue.push(to);
    }
  }
  // Edges that skip layers get a virtual node in each layer they cross,
  // so they are ordered like real nodes and drawn around them.
  const pred = new Map(ids.map((id) => [id, []]));
  const succ = new Map(ids.map((id) => [id, []]));
  const bendIDs = new Map();
  for (const [from, list] of forward) {
    for (const to of list) {
      let prev = from;
      const chain = [];
      for (let l = layerOf.get(from) + 1; l < layerOf.get(to); l++) {
        const v = `\u0000${from}\u0000${to}\u0000${l}`;
        layerOf.set(v, l);
        pred.set(v, [prev]);
        succ.set(v, []);
        succ.get(prev).push(v);
        chain.push(v);
        prev = v;
      }
      succ.get(prev).push(to);
      pred.get(to).push(prev);
      if (chain.length) bendIDs.set(`${from}\n${to}`, chain);
    }
  }
  const layers = [];
  for (const id of [...layerOf.keys()].sort()) (layers[layerOf.get(id)] ||= []).push(id);
  for (let l = 0; l < layers.length; l++) layers[l] ||= [];

  // Order each layer by the barycentre of its neighbours in the layer
  // before (sweeping right) or after (sweeping left).
  const position = new Map();
  const index = () => layers.forEach((layer) => layer.forEach((id, i) => position.set(id, i)));
  index();
  const reorder = (layer, neighbours) => {
    const key = new Map(layer.map((id) => {
      const ns = neighbours(id);
      return [id, ns.length ? ns.reduce((sum, n) => sum + position.get(n), 0) / ns.length : position.get(id)];
    }));
    layer.sort((a, b) => key.get(a) - key.get(b) || (a < b ? -1 : 1));
  };
  for (let s = 0; s < sweeps; s++) {
    for (let l = 1; l < layers.length; l++) { reorder(layers[l], (id) => pred.get(id)); index(); }
    for (let l = layers.length - 2; l >= 0; l--) { reorder(layers[l], (id) => succ.get(id)); index(); }
  }

  const all = new Map();
  layers.forEach((layer, l) => layer.forEach((id, i) => all.set(id, {
    x: l * (nodeWidth + gapX), y: i * (nodeHeight + gapY), layer: l, order: i,
  })));
  const positions = new Map(ids.map((id) => [id, all.get(id)]));
  const bends = new Map([...bendIDs].map(([key, chain]) => [key, chain.map((v) => all.get(v))]));
  const tallest = Math.max(0, ...layers.map((layer) => layer.length));
  return {
    positions, bends,
    layers: layers.map((layer) => layer.filter((id) => known.has(id))),
    width: layers.length ? layers.length * (nodeWidth + gapX) - gapX : 0,
    height: tallest ? tallest * (nodeHeight + gapY) - gapY : 0,
    nodeWidth, nodeHeight,
  };
}

/** Counts edge crossings between neighbouring layers, for tests. */
export function crossings(result, edges) {
  let count = 0;
  const pos = result.positions;
  const spans = edges.filter((e) => pos.has(e.from) && pos.has(e.to) && pos.get(e.to).layer === pos.get(e.from).layer + 1);
  for (let i = 0; i < spans.length; i++) {
    for (let j = i + 1; j < spans.length; j++) {
      const a = spans[i], b = spans[j];
      if (pos.get(a.from).layer !== pos.get(b.from).layer) continue;
      const d1 = pos.get(a.from).order - pos.get(b.from).order;
      const d2 = pos.get(a.to).order - pos.get(b.to).order;
      if (d1 * d2 < 0) count++;
    }
  }
  return count;
}
