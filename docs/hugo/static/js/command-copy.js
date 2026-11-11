/* Progressive enhancement for executable command blocks and copyable prompts. */
(() => {
  'use strict';
  const content = document.querySelector('#kube-component');
  if (!content || content.dataset.commandCopyReady) return;
  content.dataset.commandCopyReady = 'true';
  const lang = (document.documentElement.lang || 'en').toLowerCase();
  const labels = lang.startsWith('zh') ? {
    copy: '复制', copied: '已复制', error: '复制失败，请手动复制',
    prompt: '复制提示词',
    block: number => `复制第 ${number} 段命令`,
  } : lang.startsWith('ja') ? {
    copy: 'コピー', copied: 'コピーしました', error: 'コピーに失敗しました。手動でコピーしてください',
    prompt: 'プロンプトをコピー',
    block: number => `${number} 番目のコマンドブロックをコピー`,
  } : lang.startsWith('ko') ? {
    copy: '복사', copied: '복사됨', error: '복사하지 못했습니다. 직접 복사하세요',
    prompt: '프롬프트 복사',
    block: number => `${number}번째 명령 블록 복사`,
  } : lang.startsWith('es') ? {
    copy: 'Copiar', copied: '¡Copiado!', error: 'No se pudo copiar; seleccione y copie manualmente',
    prompt: 'Copiar el prompt',
    block: number => `Copiar el bloque de comandos ${number}`,
  } : {
    copy: 'Copy', copied: 'Copied!', error: 'Copy failed; select and copy manually',
    prompt: 'Copy prompt',
    block: number => `Copy command block ${number}`,
  };
  const status = document.createElement('span');
  status.className = 'command-copy-status';
  status.setAttribute('role', 'status');
  status.setAttribute('aria-live', 'polite');
  content.append(status);

  function legacyCopy(text) {
    if (typeof document.execCommand !== 'function') return false;
    const active = document.activeElement;
    const selection = window.getSelection();
    const ranges = selection ? Array.from({length: selection.rangeCount}, (_, i) => selection.getRangeAt(i).cloneRange()) : [];
    const input = document.createElement('textarea');
    input.className = 'command-copy-input';
    input.value = text;
    input.readOnly = true;
    input.tabIndex = -1;
    input.setAttribute('aria-hidden', 'true');
    document.body.append(input);
    try {
      input.focus({preventScroll: true});
      input.select();
      input.setSelectionRange(0, text.length);
      return document.execCommand('copy');
    } finally {
      input.remove();
      if (active && active.isConnected) active.focus({preventScroll: true});
      if (selection) {
        selection.removeAllRanges();
        ranges.forEach(range => selection.addRange(range));
      }
    }
  }

  async function copy(text) {
    if (navigator.clipboard && typeof navigator.clipboard.writeText === 'function') {
      try {
        await navigator.clipboard.writeText(text);
        return true;
      } catch (_) {
        // Older contexts and denied async clipboard access can use selection copy.
      }
    }
    return legacyCopy(text);
  }

  // Bash blocks are executable commands; text blocks are the copyable prompt.
  content.querySelectorAll('pre > code.language-bash, pre > code.language-text').forEach((code, index) => {
    const block = code.closest('.highlight');
    if (!block || block.querySelector('.command-copy-button')) return;
    const isPrompt = code.classList.contains('language-text');
    const label = isPrompt ? labels.prompt : labels.block(index + 1);
    block.classList.add('command-copy');
    if (isPrompt) block.classList.add('command-copy-prompt');
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'command-copy-button';
    button.setAttribute('aria-label', label);
    button.innerHTML = '<svg class="copy-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" aria-hidden="true"><rect x="8" y="8" width="12" height="13" rx="2"/><path d="M16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3"/></svg><svg class="copied-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="m5 12 4 4 10-10"/></svg>';
    const tooltip = document.createElement('span');
    tooltip.className = 'copy-tooltip';
    tooltip.setAttribute('aria-hidden', 'true');
    tooltip.textContent = isPrompt ? labels.prompt : labels.copy;
    button.append(tooltip);
    let busy = false;
    let reset;
    button.addEventListener('click', async () => {
      if (busy) return;
      busy = true;
      clearTimeout(reset);
      status.textContent = '';
      try {
        const copied = await copy(code.textContent);
        if (!copied) throw new Error('Clipboard unavailable');
        button.dataset.state = 'copied';
        tooltip.textContent = labels.copied;
        status.textContent = labels.copied;
      } catch (_) {
        button.dataset.state = 'error';
        tooltip.textContent = labels.error;
        status.textContent = labels.error;
        const range = document.createRange();
        range.selectNodeContents(code);
        const selection = window.getSelection();
        if (selection) {
          selection.removeAllRanges();
          selection.addRange(range);
        }
      } finally {
        busy = false;
        reset = setTimeout(() => {
          delete button.dataset.state;
          tooltip.textContent = isPrompt ? labels.prompt : labels.copy;
        }, 2200);
      }
    });
    block.append(button);
  });
})();
