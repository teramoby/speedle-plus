/* Inline workflow Python must not import from the checkout or runner-local paths.
 *
 * Every inline invocation (-c or stdin code) must pass -I and must begin with
 * `import sys; assert sys.flags.isolated, ...`, so the script itself enforces
 * the flag at run time; a python3 without -I support fails the step. Module
 * (-m) invocations must pass -I as well but cannot carry the inline assertion,
 * because their code comes from the module. Script files, --version and
 * --help resolve code through sys.path on purpose and are outside this guard. */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');

const workflows = path.resolve(__dirname, '../../.github/workflows');
const controls = new Set(['\n', ';', ';;', ';&', ';;&', '&&', '||', '|', '|&', '&', '(', ')']);
const redirects = new Set(['<', '<<', '<<-', '<<<', '>', '>>', '<>', '<&', '>&', '>|', '&>', '&>>']);
const operators = [...new Set([...controls, ...redirects])].sort((a, b) => b.length - a.length);
const blockScalars = /^([|>])((?:[1-9][+-]?|[+-][1-9]?))?[ \t]*(?:#.*)?$/;

function indentOf(line) {
  return line.match(/^ */)[0].length;
}

function blank(line) {
  const text = line.trim();
  return !text || text.startsWith('#');
}

function isItem(line) {
  return /^ *-(?:[ \t]|$)/.test(line);
}

// The column where the keys of a `steps:` sequence item start, or null when
// the item is not a mapping (for example a bare nested sequence).
function itemKeyIndent(lines, start, dashIndent) {
  const dash = lines[start].match(/^ *-[ \t]*/)[0];
  const rest = lines[start].slice(dash.length).trim();
  if (rest.startsWith('{') || rest.startsWith('[')) {
    throw new Error(`unsupported flow-style step at line ${start + 1}`);
  }
  if (rest && !rest.startsWith('#')) return dash.length;
  for (let index = start + 1; index < lines.length; index++) {
    if (blank(lines[index])) continue;
    const indent = indentOf(lines[index]);
    if (indent <= dashIndent) return null;
    return isItem(lines[index]) ? null : indent;
  }
  return null;
}

// Locate the items of every block-style `steps:` sequence, so a `run:` key in
// unrelated YAML (top-level data, job keys, a step's `with:`/`env:` mapping)
// is never mistaken for shell.
function stepItems(lines) {
  const items = [];
  for (let index = 0; index < lines.length; index++) {
    if (!/^ *steps:/.test(lines[index])) continue;
    if (!/^ *steps:[ \t]*(?:#.*)?$/.test(lines[index])) {
      throw new Error(`unsupported steps layout at line ${index + 1}`);
    }
    const stepsIndent = indentOf(lines[index]);
    let cursor = index + 1;
    while (cursor < lines.length && blank(lines[cursor])) cursor++;
    if (!isItem(lines[cursor] ?? '')) continue;
    const dashIndent = indentOf(lines[cursor]);
    if (dashIndent < stepsIndent) continue;
    while (cursor < lines.length) {
      const line = lines[cursor];
      if (blank(line)) {cursor++; continue;}
      const indent = indentOf(line);
      if (indent !== dashIndent || !isItem(line)) {
        if (indent <= dashIndent) break;
        cursor++;
        continue;
      }
      let end = cursor + 1;
      while (end < lines.length && (blank(lines[end]) || indentOf(lines[end]) > dashIndent)) end++;
      items.push({start: cursor, end: end - 1, keyIndent: itemKeyIndent(lines, cursor, dashIndent)});
      cursor = end;
    }
  }
  return items;
}

function runScripts(source) {
  const lines = source.split('\n');
  const scripts = [];
  for (const item of stepItems(lines)) {
    if (item.keyIndent === null) continue;
    for (let index = item.start; index <= item.end; index++) {
      let column = indentOf(lines[index]);
      let text = lines[index].slice(column);
      if (index === item.start) {
        const dash = text.match(/^-[ \t]+/);
        if (dash) {text = text.slice(dash[0].length); column += dash[0].length;}
      }
      if (column !== item.keyIndent) continue;
      const match = text.match(/^run:[ \t]*(.*)$/);
      if (!match) continue;
      const value = match[1];
      const block = value.match(blockScalars);
      if (!block) {
        let script = value;
        if (script.startsWith("'") && script.endsWith("'")) script = script.slice(1, -1).replaceAll("''", "'");
        else if (script.startsWith('"') && script.endsWith('"')) script = JSON.parse(script);
        scripts.push({line: index + 1, source: script});
        continue;
      }
      const body = [];
      let end = index + 1;
      for (; end < lines.length; end++) {
        const line = lines[end];
        if (!line.trim()) {body.push(''); continue;}
        if (indentOf(line) <= item.keyIndent) break;
        body.push(line);
      }
      const digit = (block[2] || '').match(/[1-9]/);
      const first = body.find(line => line.trim());
      const indent = digit ? item.keyIndent + Number(digit[0]) : first ? indentOf(first) : item.keyIndent + 2;
      const content = body.map(line => line.trim() ? line.slice(indent) : '');
      scripts.push({line: index + 2, source: content.join(block[1] === '|' ? '\n' : ' ')});
      index = end - 1;
    }
  }
  return scripts;
}

// Tokenize the shell subset used by workflow steps. In particular, quoted
// separators remain data and heredoc bodies never become shell commands.
function lexShell(source) {
  const tokens = [];
  const heredocs = [];
  let awaitingDelimiter = null;
  let index = 0;
  let line = 1;
  const operatorAt = () => operators.find(operator => source.startsWith(operator, index));
  while (index < source.length) {
    const character = source[index];
    if (character === ' ' || character === '\t' || character === '\r') {index++; continue;}
    if (character === '\\' && source[index + 1] === '\n') {index += 2; line++; continue;}
    if (character === '\n') {
      tokens.push({type: 'operator', value: '\n', line});
      index++; line++;
      for (const heredoc of heredocs) {
        if (heredoc.delimiter === null) continue;
        const body = [];
        while (index < source.length) {
          const end = source.indexOf('\n', index);
          const lineEnd = end === -1 ? source.length : end;
          let text = source.slice(index, lineEnd).replace(/\r$/, '');
          if (heredoc.stripTabs) text = text.replace(/^\t+/, '');
          index = end === -1 ? source.length : end + 1;
          if (end !== -1) line++;
          if (text === heredoc.delimiter) break;
          body.push(text);
        }
        heredoc.token.body = body.join('\n');
      }
      heredocs.length = 0;
      awaitingDelimiter = null;
      continue;
    }
    if (character === '#') {
      while (index < source.length && source[index] !== '\n') index++;
      continue;
    }
    const operator = operatorAt();
    if (operator) {
      const token = {type: 'operator', value: operator, line};
      tokens.push(token);
      if (operator === '<<' || operator === '<<-') {
        awaitingDelimiter = {delimiter: null, stripTabs: operator === '<<-', token};
        heredocs.push(awaitingDelimiter);
      }
      index += operator.length;
      continue;
    }
    const startLine = line;
    let value = '';
    let started = false;
    while (index < source.length) {
      const current = source[index];
      if (current === ' ' || current === '\t' || current === '\r' || current === '\n' || operatorAt()) break;
      started = true;
      if (current === '\\') {
        if (source[index + 1] === '\n') {index += 2; line++; continue;}
        if (index + 1 < source.length) {value += source[index + 1]; index += 2;} else index++;
        continue;
      }
      if (current === "'" || current === '"') {
        const quote = current;
        index++;
        while (index < source.length && source[index] !== quote) {
          if (source[index] === '\n') line++;
          if (quote === '"' && source[index] === '\\' && index + 1 < source.length) {
            const escaped = source[index + 1];
            if (escaped === '\n') {index += 2; line++; continue;}
            if ('"\\$`'.includes(escaped)) {value += escaped; index += 2; continue;}
          }
          value += source[index++];
        }
        if (source[index] === quote) index++;
        continue;
      }
      value += current;
      index++;
    }
    if (!started) throw new Error(`shell lexer made no progress at line ${line}`);
    const token = {type: 'word', value, line: startLine};
    tokens.push(token);
    if (awaitingDelimiter) {awaitingDelimiter.delimiter = value; awaitingDelimiter = null;}
  }
  return tokens;
}

function commands(tokens) {
  const result = [];
  let command = [];
  for (const token of tokens) {
    if (token.type === 'operator' && controls.has(token.value)) {
      if (command.length) result.push(command);
      command = [];
    } else command.push(token);
  }
  if (command.length) result.push(command);
  return result;
}

function redirectLength(tokens, index) {
  let operator = index;
  if (tokens[index]?.type === 'word' && /^\d+$/.test(tokens[index].value)) operator++;
  if (tokens[operator]?.type !== 'operator' || !redirects.has(tokens[operator].value)) return 0;
  return operator - index + 1 + (tokens[operator + 1]?.type === 'word' ? 1 : 0);
}

const isolationAssertion = 'import sys; assert sys.flags.isolated';
const isolationAssertionLine = /^import sys; assert sys\.flags\.isolated(?:, ['"]run with python3 -I['"])?$/;

// Heredoc bodies must start with the assertion as their own line; -c code
// continues after the statement, so only its prefix is checked.
function assertsIsolation(code, exactLine = false) {
  if (typeof code !== 'string') return false;
  if (exactLine) {
    const first = code.split('\n').map(line => line.trim()).find(Boolean);
    return first !== undefined && isolationAssertionLine.test(first);
  }
  return code.trimStart().startsWith(isolationAssertion);
}

function inspectPython(tokens, index, findings) {
  let isolated = false;
  const interpreter = tokens[index];
  // The last heredoc redirect of this command feeds Python's stdin.
  const heredoc = tokens.filter(token => token.type === 'operator' && (token.value === '<<' || token.value === '<<-')).pop();
  const operand = current => tokens[current + 1]?.type === 'word' ? tokens[current + 1].value : '';
  const report = (kind, asserted) => findings.push({line: interpreter.line, kind, isolated, asserted});
  for (let current = index + 1; current < tokens.length;) {
    // Redirections never decide what Python executes; skip them and their
    // operand so a script file with redirected input stays a file execution.
    const length = redirectLength(tokens, current);
    if (length) {current += length; continue;}
    const token = tokens[current];
    if (token.type !== 'word') {current++; continue;}
    const value = token.value;
    if (value === '-') {
      report('stdin', assertsIsolation(heredoc?.body, true));
      return;
    }
    if (value.startsWith('-c')) {
      report('command', assertsIsolation(value.length > 2 ? value.slice(2) : operand(current)));
      return;
    }
    const clustered = value.match(/^-([BbEhiIOoPqRsStuUvVx]*)(c)$/);
    if (clustered) {
      isolated = isolated || clustered[1].includes('I');
      report('command', assertsIsolation(operand(current)));
      return;
    }
    const moduleOption = value.match(/^-([BbEhiIOoPqRsStuUvVx]*)(m)$/);
    if (moduleOption || value.startsWith('-m')) {
      if (moduleOption) isolated = isolated || moduleOption[1].includes('I');
      report('module', false);
      return;
    }
    if (/^-[BbEhiIOoPqRsStuUvVx]+$/.test(value)) {
      isolated = isolated || value.includes('I');
      current++;
      continue;
    }
    if (value === '-X' || value === '-W' || value === '--check-hash-based-pycs') {current += 2; continue;}
    if (value === '--version' || value === '--help') return;
    if (value.startsWith('-')) {current++; continue;}
    // An expansion followed by command-string, module or stdin arguments could
    // be flags supplied through a variable; fail closed instead of assuming a
    // script file.
    if (/[$`]/.test(value)) {
      if (hasInlinePythonArguments(tokens, current)) report('indirect', false);
      return;
    }
    // A script file legitimately imports from its own directory.
    return;
  }
  // No script argument: Python executes its standard input.
  report('stdin', assertsIsolation(heredoc?.body, true));
}

function inspectSource(source, findings, depth = 0) {
  if (depth > 20) throw new Error('nested shell command limit exceeded');
  for (const command of commands(lexShell(source))) inspectCommand(command, findings, depth);
}

function inspectShell(command, index, findings, depth) {
  for (let current = index + 1; current < command.length;) {
    const length = redirectLength(command, current);
    if (length) {current += length; continue;}
    const token = command[current];
    if (token.type !== 'word') {current++; continue;}
    const value = token.value;
    let code;
    if (value.startsWith('-c')) code = value.slice(2);
    else if (/^-[A-Za-z]+c$/.test(value)) code = '';
    else if (value === '--') return;
    else if (value.startsWith('-')) {current++; continue;}
    else return;
    const argument = command[current + 1];
    if (!code && argument?.type === 'word') code = argument.value;
    if (code) inspectSource(code, findings, depth + 1);
    return;
  }
}

// Arguments that only an inline-Python or shell invocation takes: a command
// string option, a lone dash (stdin) or a heredoc redirect.
function hasInlinePythonArguments(command, index) {
  for (let current = index + 1; current < command.length; current++) {
    const token = command[current];
    if (token.type === 'operator') {
      if (token.value === '<<' || token.value === '<<-') return true;
      continue;
    }
    const value = token.value;
    if (value === '-' || value === '-c' || value.startsWith('-c')) return true;
    if (value === '-m' || value.startsWith('-m')) return true;
    if (/^-[BbEhiIOoPqRsStuUvVx]*[cm]$/.test(value)) return true;
  }
  return false;
}

function inspectCommand(command, findings, depth) {
  const prefixes = new Set(['if', 'elif', 'while', 'until', 'then', 'else', 'do', '!']);
  const optionArguments = {
    env: new Set(['-u', '--unset', '-C', '--chdir', '-S', '--split-string']),
    sudo: new Set(['-u', '--user', '-g', '--group', '-h', '--host', '-p', '--prompt', '-C', '-T', '-t', '-D', '-R']),
  };
  let wrapper = '';
  for (let index = 0; index < command.length;) {
    const length = redirectLength(command, index);
    if (length) {index += length; continue;}
    const token = command[index];
    if (token.type !== 'word') {index++; continue;}
    const value = token.value;
    const name = value.slice(value.lastIndexOf('/') + 1);
    if (!wrapper && (prefixes.has(value) || /^[A-Za-z_][A-Za-z0-9_]*=/.test(value))) {index++; continue;}
    if (wrapper && (value === '-v' || value === '-V') && wrapper === 'command') return;
    if (wrapper && /^[A-Za-z_][A-Za-z0-9_]*=/.test(value)) {index++; continue;}
    if (wrapper && value.startsWith('-')) {index += optionArguments[wrapper]?.has(value) ? 2 : 1; continue;}
    // An indirected command with inline-Python arguments could hide an
    // interpreter; fail closed instead of assuming it is not Python.
    if (/[$`]/.test(value) && hasInlinePythonArguments(command, index)) {
      findings.push({line: token.line, kind: 'indirect', isolated: false, asserted: false});
      return;
    }
    if (/^python(?:3(?:\.[0-9]+)?)?$/.test(name)) {inspectPython(command, index, findings); return;}
    if (name === 'bash' || name === 'sh') {inspectShell(command, index, findings, depth); return;}
    if (['env', 'sudo', 'nohup', 'command', 'time'].includes(name)) {wrapper = name; index++; continue;}
    return;
  }
}

function inlinePythonInvocations(source) {
  const findings = [];
  for (const script of runScripts(source)) {
    const nested = [];
    inspectSource(script.source, nested);
    findings.push(...nested.map(finding => ({...finding, line: script.line + finding.line - 1})));
  }
  return findings;
}

test('every inline Python invocation in every workflow uses isolated mode', () => {
  const files = fs.readdirSync(workflows).filter(name => /\.ya?ml$/.test(name)).sort();
  assert.ok(files.length > 0, 'no workflows discovered');
  const invocations = files.flatMap(file => inlinePythonInvocations(fs.readFileSync(path.join(workflows, file), 'utf8'))
    .map(invocation => ({file, ...invocation})));
  const expectedCount = 13;
  assert.equal(invocations.length, expectedCount, 'discovered invocation count must match the expected table');
  assert.deepEqual(invocations.filter(invocation => !invocation.isolated), []);
  assert.deepEqual(invocations.filter(invocation => invocation.kind !== 'module' && !invocation.asserted), []);

  const expected = [
    ['ci.yml', 'module'],
    ['ci.yml', 'stdin'],
    ['go-ci-worker.yml', 'command'],
    ['go-ci-worker.yml', 'command'],
    ['go-ci-worker.yml', 'stdin'],
    ['go-ci-worker.yml', 'stdin'],
    ['go-ci-worker.yml', 'stdin'],
    ['go-ci-worker.yml', 'stdin'],
    ['image-smoke.yml', 'stdin'],
    ['mongodb-nightly-validation.yml', 'command'],
    ['mongodb-nightly-validation.yml', 'command'],
    ['mongodb-nightly-validation.yml', 'command'],
    ['mongodb-nightly-validation.yml', 'stdin'],
  ];
  const remaining = [...invocations];
  for (const [file, kind] of expected) {
    const index = remaining.findIndex(invocation => invocation.file === file && invocation.kind === kind);
    assert.notEqual(index, -1, `missing expected ${kind} invocation in ${file}`);
    remaining.splice(index, 1);
  }
  assert.deepEqual(remaining, [], 'unexpected inline Python invocation; update the expected list');
});

function workflowWithSteps(steps) {
  return `name: fixture\non: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n${steps
    .map(step => step.split('\n').map(line => `      ${line}`).join('\n')).join('\n')}\n`;
}

function blockStep(script, header = '|') {
  return `- run: ${header}\n${script.split('\n').map(line => `    ${line}`).join('\n')}`;
}

function workflowWithRuns(scripts) {
  return workflowWithSteps(scripts.map(script => blockStep(script)));
}

test('the guard covers interpreter spellings, paths, wrappers and heredoc forms', () => {
  const assertLine = 'import sys; assert sys.flags.isolated';
  const source = workflowWithSteps([
    blockStep("python3 - <<'PY'\nprint('stdin')\nPY"),
    blockStep(`python3 -I - <<'PY'\n${assertLine}\nprint('stdin')\nPY`),
    blockStep("python3 <<'PY'\nprint('stdin')\nPY"),
    blockStep(`python3 -I <<'PY'\n${assertLine}\nprint('stdin')\nPY`),
    blockStep("python3 -I <<'PY'\nprint('stdin')\nPY"),
    blockStep("python <<'PY'\nprint('stdin')\nPY"),
    "- run: python3 -c'print(1)'",
    `- run: /usr/bin/python3 -I -c '${assertLine}; print(1)'`,
    "- run: env python3 -c 'print(1)'",
    `- run: /usr/bin/env python3 -I -c '${assertLine}; print(1)'`,
    `- run: sudo python3 -I -c '${assertLine}; print(1)'`,
    `- run: bash -c "python3 -I -c '${assertLine}; print(1)'"`,
    `- run: FOO="a b" python3 -I -c '${assertLine}; print(1)'`,
    "- run: python3 -c 'print(1)' 2>/dev/null",
    `- run: python3 -Ic '${assertLine}; print(1)'`,
    "- run: python3 -I -c 'print(1)'",
    "- run: python -c 'print(1)'",
    `- run: python -I -c '${assertLine}; print(1)'`,
    "- run: python3.12 -c 'print(1)'",
    `- run: $PY -I -c '${assertLine}'`,
    `- run: \`python3\` -I -c '${assertLine}'`,
    blockStep(`python3 -I <<'PY'\n${assertLine}; sys.path.insert(0, '.')\nPY`),
    '- run: python3 -m unittest discover -s scripts/tests',
    '- run: python3 -I -m unittest discover -s scripts/tests',
    '- run: python3 -Im unittest discover -s scripts/tests',
    '- run: "$PY" -m unittest discover -s scripts/tests',
    `- run: python3 $FLAGS -c '${assertLine}'`,
  ]);
  assert.deepEqual(inlinePythonInvocations(source).map(({kind, isolated, asserted}) => ({kind, isolated, asserted})), [
    {kind: 'stdin', isolated: false, asserted: false},
    {kind: 'stdin', isolated: true, asserted: true},
    {kind: 'stdin', isolated: false, asserted: false},
    {kind: 'stdin', isolated: true, asserted: true},
    {kind: 'stdin', isolated: true, asserted: false},
    {kind: 'stdin', isolated: false, asserted: false},
    {kind: 'command', isolated: false, asserted: false},
    {kind: 'command', isolated: true, asserted: true},
    {kind: 'command', isolated: false, asserted: false},
    {kind: 'command', isolated: true, asserted: true},
    {kind: 'command', isolated: true, asserted: true},
    {kind: 'command', isolated: true, asserted: true},
    {kind: 'command', isolated: true, asserted: true},
    {kind: 'command', isolated: false, asserted: false},
    {kind: 'command', isolated: true, asserted: true},
    {kind: 'command', isolated: true, asserted: false},
    {kind: 'command', isolated: false, asserted: false},
    {kind: 'command', isolated: true, asserted: true},
    {kind: 'command', isolated: false, asserted: false},
    {kind: 'indirect', isolated: false, asserted: false},
    {kind: 'indirect', isolated: false, asserted: false},
    {kind: 'stdin', isolated: true, asserted: false},
    {kind: 'module', isolated: false, asserted: false},
    {kind: 'module', isolated: true, asserted: false},
    {kind: 'module', isolated: true, asserted: false},
    {kind: 'indirect', isolated: false, asserted: false},
    {kind: 'indirect', isolated: false, asserted: false},
  ]);
});

test('the guard ignores non-run YAML, quoted data, comments, heredoc bodies and file execution', () => {
  const source = workflowWithRuns([
    "echo 'log; python3 -c \"not a command\"'",
    '# (python3 -c "comment only")',
    "python3 <<'PY'\npython3 -c 'heredoc body, not a shell command'\nPY",
    'python3 scripts/ci/test_report.py -c',
    'python3 scripts/ci/run.py < input.txt',
    'python3 < input.txt scripts/ci/run.py',
    'command -v python3 >/dev/null',
    'bash --version; python3 --version',
    'for tool in bash python3; do command -v "$tool"; done',
    "bash -c \"echo 'python3 -c quoted text'\"",
  ]);
  assert.deepEqual(inlinePythonInvocations(source).map(({kind, isolated, asserted}) => ({kind, isolated, asserted})), [
    {kind: 'stdin', isolated: false, asserted: false},
  ]);
});

test('the guard reads only run keys that belong to workflow steps', () => {
  const source = `name: fixture
on: push
env:
  run: python3 -c 'top-level data'
jobs:
  test:
    runs-on: ubuntu-latest
    run: python3 -c 'job-level data'
    steps:
      - name: step data
        with:
          run: python3 -c 'step input, not a command'
        env:
          run: python3 -c 'step environment, not a command'
        run: |
          python3 -I -c 'import sys; assert sys.flags.isolated; print(1)'
      -
        run: python3 -I -c 'import sys; assert sys.flags.isolated; print(1)'
    run: python3 -c 'job data after the step list'
`;
  assert.deepEqual(inlinePythonInvocations(source).map(({kind, isolated, asserted}) => ({kind, isolated, asserted})), [
    {kind: 'command', isolated: true, asserted: true},
    {kind: 'command', isolated: true, asserted: true},
  ]);
});

test('the guard reads standard block scalar headers', () => {
  const assertLine = 'import sys; assert sys.flags.isolated';
  const source = workflowWithSteps([
    blockStep(`python3 -I -c '${assertLine}; print(1)'`, '|-'),
    blockStep(`python3 -I -c '${assertLine}; print(1)'`, '|+'),
    blockStep(`python3 -I -c '${assertLine}; print(1)'`, '>-'),
    blockStep(`python3 -I -c '${assertLine}; print(1)'`, '|2'),
    blockStep(`python3 -I -c '${assertLine}; print(1)'`, '|-2'),
    blockStep(`python3 -I -c '${assertLine}; print(1)'`, '| # comment'),
  ]);
  assert.deepEqual(inlinePythonInvocations(source).map(({kind}) => kind), [
    'command', 'command', 'command', 'command', 'command', 'command',
  ]);
});

test('the guard fails closed on unsupported step layouts', () => {
  assert.throws(() => inlinePythonInvocations('name: fixture\njobs:\n  test:\n    steps: [{run: "python3 -c \'print(1)\'"}]\n'),
    /unsupported steps layout/);
  assert.throws(() => inlinePythonInvocations('name: fixture\njobs:\n  test:\n    steps: &steps\n      - run: echo hi\n'),
    /unsupported steps layout/);
  assert.throws(() => inlinePythonInvocations('name: fixture\njobs:\n  test:\n    steps:\n      - {run: "python3 -c \'print(1)\'"}\n'),
    /unsupported flow-style step/);
});
