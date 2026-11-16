/* Inline workflow Python must not import from the checkout or runner-local paths.
 *
 * Every inline invocation must pass -I. On Python 3.11+ -I also implies -P,
 * which keeps the working directory off sys.path for -c and stdin code; on
 * 3.9/3.10 it does not, so these steps assume a runner python3 >= 3.11
 * (ubuntu-latest ships 3.12+). */
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
  if (lines[start].slice(dash.length).trim()) return dash.length;
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
    if (!/^ *steps:[ \t]*(?:#.*)?$/.test(lines[index])) continue;
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
        while (index < source.length) {
          const end = source.indexOf('\n', index);
          const lineEnd = end === -1 ? source.length : end;
          let text = source.slice(index, lineEnd).replace(/\r$/, '');
          if (heredoc.stripTabs) text = text.replace(/^\t+/, '');
          index = end === -1 ? source.length : end + 1;
          if (end !== -1) line++;
          if (text === heredoc.delimiter) break;
        }
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
      tokens.push({type: 'operator', value: operator, line});
      if (operator === '<<' || operator === '<<-') {
        awaitingDelimiter = {delimiter: null, stripTabs: operator === '<<-'};
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

function inspectPython(tokens, index, findings) {
  let isolated = false;
  const interpreter = tokens[index];
  for (let current = index + 1; current < tokens.length;) {
    // Redirections never decide what Python executes; skip them and their
    // operand so a script file with redirected input stays a file execution.
    const length = redirectLength(tokens, current);
    if (length) {current += length; continue;}
    const token = tokens[current];
    if (token.type !== 'word') {current++; continue;}
    const value = token.value;
    if (value === '-') {
      findings.push({line: interpreter.line, kind: 'stdin', isolated});
      return;
    }
    if (value.startsWith('-c')) {
      findings.push({line: interpreter.line, kind: 'command', isolated});
      return;
    }
    const clustered = value.match(/^-([BbEhiIOoPqRsStuUvVx]*)(c)$/);
    if (clustered) {
      if (clustered[1].includes('I')) isolated = true;
      findings.push({line: interpreter.line, kind: 'command', isolated});
      return;
    }
    if (/^-[BbEhiIOoPqRsStuUvVx]+$/.test(value)) {
      if (value.includes('I')) isolated = true;
      current++;
      continue;
    }
    if (value === '-X' || value === '-W' || value === '--check-hash-based-pycs') {current += 2; continue;}
    if (value === '-m' || value === '--version' || value === '--help') return;
    if (value.startsWith('-')) {current++; continue;}
    // A script file legitimately imports from its own directory.
    return;
  }
  // No script argument: Python executes its standard input.
  findings.push({line: interpreter.line, kind: 'stdin', isolated});
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
  assert.deepEqual(invocations.filter(invocation => !invocation.isolated), []);

  const expected = [
    ['ci.yml', 'stdin'],
    ['go-ci-worker.yml', 'stdin'],
    ['go-ci-worker.yml', 'stdin'],
    ['go-ci-worker.yml', 'stdin'],
    ['image-smoke.yml', 'stdin'],
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
  const source = workflowWithSteps([
    blockStep("python3 - <<'PY'\nprint('stdin')\nPY"),
    blockStep("python3 -I - <<'PY'\nprint('stdin')\nPY"),
    blockStep("python3 <<'PY'\nprint('stdin')\nPY"),
    blockStep("python3 -I <<'PY'\nprint('stdin')\nPY"),
    blockStep("python3 -I<<'PY'\nprint('stdin')\nPY"),
    blockStep("python <<'PY'\nprint('stdin')\nPY"),
    "- run: python3 -c'print(1)'",
    "- run: /usr/bin/python3 -I -c 'print(1)'",
    "- run: env python3 -c 'print(1)'",
    "- run: /usr/bin/env python3 -I -c 'print(1)'",
    "- run: sudo python3 -I -c 'print(1)'",
    "- run: bash -c \"python3 -c 'print(1)'\"",
    "- run: FOO=\"a b\" python3 -I -c 'print(1)'",
    "- run: python3 -c 'print(1)' 2>/dev/null",
    "- run: python3 -I < input.txt",
    "- run: python -c 'print(1)'",
    "- run: python -I -c 'print(1)'",
    "- run: python3.12 -c 'print(1)'",
  ]);
  assert.deepEqual(inlinePythonInvocations(source).map(({kind, isolated}) => ({kind, isolated})), [
    {kind: 'stdin', isolated: false},
    {kind: 'stdin', isolated: true},
    {kind: 'stdin', isolated: false},
    {kind: 'stdin', isolated: true},
    {kind: 'stdin', isolated: true},
    {kind: 'stdin', isolated: false},
    {kind: 'command', isolated: false},
    {kind: 'command', isolated: true},
    {kind: 'command', isolated: false},
    {kind: 'command', isolated: true},
    {kind: 'command', isolated: true},
    {kind: 'command', isolated: false},
    {kind: 'command', isolated: true},
    {kind: 'command', isolated: false},
    {kind: 'stdin', isolated: true},
    {kind: 'command', isolated: false},
    {kind: 'command', isolated: true},
    {kind: 'command', isolated: false},
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
  assert.deepEqual(inlinePythonInvocations(source).map(({kind, isolated}) => ({kind, isolated})), [
    {kind: 'stdin', isolated: false},
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
          python3 -I -c 'real command'
      -
        run: python3 -I -c 'bare dash step'
    run: python3 -c 'job data after the step list'
`;
  assert.deepEqual(inlinePythonInvocations(source).map(({kind, isolated}) => ({kind, isolated})), [
    {kind: 'command', isolated: true},
    {kind: 'command', isolated: true},
  ]);
});

test('the guard reads standard block scalar headers', () => {
  const source = workflowWithSteps([
    blockStep("python3 -c 'strip'", '|-'),
    blockStep("python3 -c 'keep'", '|+'),
    blockStep("python3 -c 'folded'", '>-'),
    blockStep("python3 -c 'explicit indent'", '|2'),
    blockStep("python3 -c 'explicit strip'", '|-2'),
    blockStep("python3 -c 'comment'", '| # comment'),
  ]);
  assert.deepEqual(inlinePythonInvocations(source).map(({kind}) => kind), [
    'command', 'command', 'command', 'command', 'command', 'command',
  ]);
});
