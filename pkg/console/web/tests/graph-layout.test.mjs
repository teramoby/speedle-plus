import assert from 'node:assert/strict';
import test from 'node:test';
import { crossings, layout } from '../static/graph-layout.js';
import { parsePrincipals } from '../static/principals.js';

const n = (...ids) => ids.map((id) => ({ id }));
const e = (...pairs) => pairs.map(([from, to]) => ({ from, to }));

test('nodes go one layer after their furthest predecessor', () => {
  const r = layout(n('user:a', 'role:x', 'role:y', 'role:z'), e(['user:a', 'role:x'], ['role:x', 'role:y'], ['user:a', 'role:y'], ['role:y', 'role:z']));
  assert.deepEqual(r.layers, [['user:a'], ['role:x'], ['role:y'], ['role:z']]);
  assert.equal(r.positions.get('role:z').x, 3 * (180 + 70));
  assert.equal(r.width, 4 * 180 + 3 * 70);
  // The skipping edge a→y bends through layer 1, beside role:x.
  assert.equal(r.bends.get('user:a\nrole:y').length, 1);
  assert.equal(r.bends.get('user:a\nrole:y')[0].layer, 1);
  assert.equal(r.height, 2 * 34 + 16);
});

test('cycles and self loops terminate and every node is placed', () => {
  const nodes = n('role:a', 'role:b', 'role:c', 'role:d', 'user:u');
  const edges = e(['role:a', 'role:b'], ['role:b', 'role:c'], ['role:c', 'role:a'], ['role:d', 'role:d'], ['user:u', 'role:a'], ['ghost', 'role:a']);
  const r = layout(nodes, edges);
  assert.equal(r.positions.size, 5);
  assert.deepEqual(r.layers.flat().sort(), nodes.map((x) => x.id).sort());
});

test('cycles are cut where they are entered', () => {
  // Sorted by name, role:b would come first and land left of role:a.
  const r = layout(n('user:u', 'role:a', 'role:b'), e(['user:u', 'role:b'], ['role:b', 'role:a'], ['role:a', 'role:b']));
  assert.deepEqual(r.layers, [['user:u'], ['role:b'], ['role:a']]);
});

test('minLayer keeps nodes right of a column', () => {
  const r = layout([{ id: 'u', kind: 'principal' }, { id: 'lonely', kind: 'role' }], [], { minLayer: (node) => (node.kind === 'role' ? 1 : 0) });
  assert.deepEqual(r.layers, [['u'], ['lonely']]);
});

test('the result does not depend on input order', () => {
  const nodes = n('u1', 'u2', 'u3', 'r1', 'r2', 'r3');
  const edges = e(['u1', 'r3'], ['u2', 'r1'], ['u3', 'r2'], ['u1', 'r1']);
  const a = layout(nodes, edges);
  const b = layout([...nodes].reverse(), [...edges].reverse());
  assert.deepEqual([...a.positions], [...b.positions]);
});

test('ordering removes avoidable crossings', () => {
  // Sorted by name, a→z and b→y cross; reordering untangles them.
  const nodes = n('a', 'b', 'y', 'z');
  const edges = e(['a', 'z'], ['b', 'y']);
  assert.equal(crossings(layout(nodes, edges, { sweeps: 0 }), edges), 1);
  assert.equal(crossings(layout(nodes, edges), edges), 0);
});

test('an empty graph has no size', () => {
  const r = layout([], []);
  assert.equal(r.width, 0);
  assert.equal(r.height, 0);
});

test('subject principals are parsed as policies write them', () => {
  assert.deepEqual(parsePrincipals('user:alice\n\n idd=corp:group:staff \nentity:svc:a'), {
    principals: [{ type: 'user', name: 'alice' }, { type: 'group', name: 'staff', idd: 'corp' }, { type: 'entity', name: 'svc:a' }],
    errors: [],
  });
  assert.deepEqual(parsePrincipals('role:admin\nalice').errors.map((x) => x.path), ['principals.0', 'principals.1']);
  assert.deepEqual(parsePrincipals(undefined), { principals: [], errors: [] });
});
