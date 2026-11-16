/* Inline workflow Python must not import from the checkout or runner-local paths. */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');

const workflows = path.resolve(__dirname, '../../.github/workflows');
const controls = new Set(['\n', ';', ';;', ';&', ';;&', '&&', '||', '|', '|&', '&', '(', ')']);
const redirects = new Set(['<', '<<', '<<-', '<<<', '>', '>>', '<>', '<&', '>&', '>|', '&>', '&>>']);
const operators = [...new Set([...controls, ...redirects])].sort((a, b) => b.length - a.length);
const stdinRedirects = new Set(['<', '<<', '<<-', '<<<']);

function runScripts(source) {
  const lines = source.split('\n');
  const scripts = [];
  for (let index = 0; index < lines.length; index++) {
    const match = lines[index].match(/^( *)(?:- )?run:[ \t]*(.*)$/);
    if (!match || !match[2]) continue;
    const keyIndent = match[1].length;
    const value = match[2];
    const block = value.match(/^([|>][+-]?)[ \t]*(?:#.*)?$/);
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
      if (line.match(/^ */)[0].length <= keyIndent) break;
      body.push(line);
    }
    const first = body.find(line => line.trim());
    const indent = first ? first.match(/^ */)[0].length : keyIndent + 2;
    const content = body.map(line => line.trim() ? line.slice(indent) : '');
    scripts.push({line: index + 2, source: content.join(block[1][0] === '|' ? '\n' : ' ')});
    index = end - 1;
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
    const length = redirectLength(tokens, current);
    if (length) {
      const operator = tokens[current].type === 'operator' ? tokens[current] : tokens[current + 1];
      if (stdinRedirects.has(operator.value)) {
        findings.push({line: interpreter.line, kind: 'stdin', isolated});
        return;
      }
      current += length;
      continue;
    }
    const token = tokens[current];
    if (token.type !== 'word') {current++; continue;}
    const value = token.value;
    if (value === '-') {
      findings.push({line: interpreter.line, kind: 'stdin', isolated});
      return;
    }
    if (value.startsWith('-c') || /^-[BbEhiIOoPqRsStuUvVx]*c/.test(value)) {
      if (!value.startsWith('-c') && value.slice(1).includes('I')) isolated = true;
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
    return;
  }
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
    if (name === 'python3') {inspectPython(command, index, findings); return;}
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

  const remaining = [...invocations];
  const expected = [
    ['ci.yml', 'stdin'],
    ['go-ci-worker.yml', 'stdin'],
    ['go-ci-worker.yml', 'stdin'],
    ['go-ci-worker.yml', 'stdin'],
    ['image-smoke.yml', 'stdin'],
    ['mongodb-nightly-validation.yml', 'command'],
    ['mongodb-nightly-validation.yml', 'stdin'],
  ];
  for (const [file, kind] of expected) {
    const index = remaining.findIndex(invocation => invocation.file === file && invocation.kind === kind);
    assert.notEqual(index, -1, `missing expected ${kind} invocation in ${file}`);
    remaining.splice(index, 1);
  }
});

function workflowWithRuns(scripts) {
  return `description: python3 -c 'not a run step'\n${scripts.map(script =>
    `      - run: |\n${script.split('\n').map(line => `          ${line}`).join('\n')}`).join('\n')}\n`;
}

test('the guard covers inline forms, paths, wrappers and quoted assignments', () => {
  const source = workflowWithRuns([
    "python3 - <<'PY'\nprint('stdin')\nPY",
    "python3 -I - <<'PY'\nprint('stdin')\nPY",
    "python3 <<'PY'\nprint('stdin')\nPY",
    "python3 -c'print(1)'",
    "/usr/bin/python3 -I -c 'print(1)'",
    "env python3 -c 'print(1)'",
    "sudo python3 -I -c 'print(1)'",
    "bash -c \"python3 -c 'print(1)'\"",
    "FOO=\"a b\" python3 -I -c 'print(1)'",
    "python3 -I<<'PY'\nprint('stdin')\nPY",
    "python3 -c 'print(1)' 2>/dev/null",
  ]) + "      - run: python3 -c 'print(1)'\n";
  assert.deepEqual(inlinePythonInvocations(source).map(({kind, isolated}) => ({kind, isolated})), [
    {kind: 'stdin', isolated: false},
    {kind: 'stdin', isolated: true},
    {kind: 'stdin', isolated: false},
    {kind: 'command', isolated: false},
    {kind: 'command', isolated: true},
    {kind: 'command', isolated: false},
    {kind: 'command', isolated: true},
    {kind: 'command', isolated: false},
    {kind: 'command', isolated: true},
    {kind: 'stdin', isolated: true},
    {kind: 'command', isolated: false},
    {kind: 'command', isolated: false},
  ]);
});

test('the guard ignores non-run YAML, quoted data, comments, heredoc bodies and file execution', () => {
  const source = workflowWithRuns([
    "echo 'log; python3 -c \"not a command\"'",
    '# (python3 -c "comment only")',
    "python3 <<'PY'\npython3 -c 'heredoc body, not a shell command'\nPY",
    'python3 scripts/ci/test_report.py -c',
    'command -v python3 >/dev/null',
    'bash --version; python3 --version',
    'for tool in bash python3; do command -v "$tool"; done',
    "bash -c \"echo 'python3 -c quoted text'\"",
  ]);
  assert.deepEqual(inlinePythonInvocations(source).map(({kind, isolated}) => ({kind, isolated})), [
    {kind: 'stdin', isolated: false},
  ]);
});
