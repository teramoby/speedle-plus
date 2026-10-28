import { requestToForm, serializeRequest } from './request-model.js';
const $ = (id) => document.getElementById(id);
const baseRequest = { subject: { principals: [{ type: 'user', name: 'alice' }] }, action: 'read', resource: 'book', serviceName: 'playground' };
const samples = {
  allow: { policies: '# Alice may read book\ngrant user alice read book\n\n# But may not delete book\ndeny user alice delete book', request: baseRequest },
  deny: { policies: '# Both grant and deny match; deny takes precedence\ngrant user alice read book\ndeny user alice read book', request: baseRequest },
  nomatch: { policies: '# The policy allows Alice, but this request is from Bob\ngrant user alice read book', request: { ...baseRequest, subject: { principals: [{ type: 'user', name: 'bob' }] } } },
  role: { policies: '# First assign Alice the reader role\ngrant user alice reader on book\n\n# Then grant the role permission to read\ngrant role reader read book', request: baseRequest },
  condition: { policies: '# Allow access only when clearance is at least 3\ngrant user alice read book if clearance >= 3', request: { ...baseRequest, attributes: [{ name: 'clearance', type: 'numeric', value: 4 }] } }
};
let engine, ready = false, busy = false, inputVersion = 0, engineErrorVisible = false, toastTimer;
const editors = [
  { input: $('policies'), highlight: $('policy-highlight'), gutter: $('policy-lines'), count: $('policy-count'), language: 'spdl', container: $('policy-editor') }
];
const escapeHTML = (s) => s.replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
function highlight(code, language) {
  const re = language === 'json' ? /"(?:\\.|[^"\\])*"(?=\s*:)|"(?:\\.|[^"\\])*"|\b(?:true|false|null)\b|-?\b\d+(?:\.\d+)?\b/g : /#[^\n]*|'(?:\\.|[^'\\])*'|\b(?:grant|deny|user|group|entity|role|if|in|on|from)\b|\b(?:true|false)\b|\b\d+(?:\.\d+)?\b/gi;
  let output = '', end = 0;
  for (const match of code.matchAll(re)) {
    output += escapeHTML(code.slice(end, match.index));
    const token = match[0];
    let type = 'keyword';
    if (token[0] === '#') type = 'comment';
    else if (token[0] === '"') type = /^\s*:/.test(code.slice(match.index + token.length)) ? 'property' : 'string';
    else if (token[0] === "'") type = 'string';
    else if (/^-?\d/.test(token)) type = 'number';
    else if (/^(true|false|null)$/i.test(token)) type = 'boolean';
    else if (/^deny$/i.test(token)) type = 'deny';
    output += `<span class="tok-${type}">${escapeHTML(token)}</span>`;
    end = match.index + token.length;
  }
  return output + escapeHTML(code.slice(end)) + '\n';
}
function renderEditor(editor) {
  editor.highlight.innerHTML = highlight(editor.input.value, editor.language);
  const lines = editor.input.value.split('\n').length;
  editor.gutter.textContent = Array.from({length:lines}, (_, n) => n + 1).join('\n');
  editor.count.textContent = `${lines} lines`;
  syncScroll(editor);
}
function syncScroll(editor) {
  editor.highlight.scrollTop = editor.input.scrollTop;
  editor.highlight.scrollLeft = editor.input.scrollLeft;
  editor.gutter.scrollTop = editor.input.scrollTop;
}
function icon(id) { $('result-symbol').replaceChildren(); const svg = document.createElementNS('http://www.w3.org/2000/svg','svg'); svg.setAttribute('class','icon'); const use=document.createElementNS('http://www.w3.org/2000/svg','use'); use.setAttribute('href',`#i-${id}`); svg.append(use); $('result-symbol').append(svg); }
function resetResult(edited = false) {
  engineErrorVisible = false;
  $('result-card').className = 'result-card idle'; icon('terminal');
  $('result-label').textContent = edited ? 'Inputs updated. Evaluate again to see the result.' : 'Ready to test your policies.';
  $('result-description').textContent = 'Select Evaluate to check whether this request is allowed.';
  $('result-type').textContent = 'BOOLEAN OUTPUT';
  $('result-meta').textContent = edited ? 'Run again' : 'Not run yet';
  $('result-details').hidden = true;
  $('result-details').open = false;
  $('result-json').textContent = '';
  editors.forEach(e => { e.container.classList.remove('invalid'); e.input.removeAttribute('aria-invalid'); });
  clearFormErrors();
}
function changed() {
  inputVersion++;
  resetResult(true);
  document.querySelectorAll('[data-sample]').forEach(b => { b.classList.remove('active'); b.setAttribute('aria-pressed','false'); });
}
for (const editor of editors) {
  editor.input.addEventListener('input', () => { renderEditor(editor); changed(); });
  editor.input.addEventListener('scroll', () => syncScroll(editor));
  editor.input.addEventListener('keydown', e => {
    if (e.key === 'Tab' && !e.shiftKey) {
      e.preventDefault();
      const {selectionStart:start, selectionEnd:end} = editor.input;
      editor.input.setRangeText('  ',start,end,'end'); renderEditor(editor); changed();
    }
  });
}
function setSample(key) {
  const sample = samples[key];
  if (!sample) return;
  $('policies').value = sample.policies;
  populateForm(requestToForm(sample.request));
  inputVersion++;
  editors.forEach(e => { e.input.scrollTop=0; e.input.scrollLeft=0; renderEditor(e); });
  resetResult();
  document.querySelectorAll('[data-sample]').forEach(b => {const selected=b.dataset.sample===key;b.classList.toggle('active',selected);b.setAttribute('aria-pressed',String(selected));});
}
function notify(message) {
  clearTimeout(toastTimer); $('toast').textContent = message; $('toast').hidden = false;
  toastTimer=setTimeout(() => {$('toast').hidden=true;},2400);
}
function showError(error, raw) {
  const stage = error.stage || 'evaluation';
  engineErrorVisible = stage === 'engine' || stage === 'initialization';
  const labels = { policies:'SPDL policy error', policy:'SPDL policy error', parse:'SPDL policy error', request:'Request format error', json:'JSON format error', condition:'Condition expression error', input:'Input validation failed', evaluation:'Evaluation error', engine:'Engine unavailable', initialization:'Engine unavailable', validation:'Input validation failed' };
  $('result-card').className = 'result-card error'; icon('alert');
  $('result-label').textContent = labels[stage] || 'Input or evaluation error';
  $('result-description').textContent = `${error.line ? `Line ${error.line} · ` : ''}${error.message || 'Unknown error. Check your inputs and try again.'}`;
  $('result-type').textContent = 'ERROR · NOT FALSE'; $('result-meta').textContent='No authorization result';
  const policyStage = /polic|parse|spdl|condition/i.test(stage), requestStage = /request|json/i.test(stage);
  editors.forEach((e,i) => { if ((i===0 && policyStage)||(i===1 && requestStage)) {e.container.classList.add('invalid');e.input.setAttribute('aria-invalid','true');} });
  if(raw) showRaw(raw); else {$('result-details').hidden=true;$('result-json').textContent='';}
}
function showRaw(data) { $('result-details').hidden = false; $('result-json').textContent = JSON.stringify(data,null,2); }
function updateButton() {
  $('evaluate').disabled = !ready || busy;
  $('evaluate-label').textContent = busy ? 'Evaluating…' : ready ? 'Evaluate' : 'Engine not ready';
}
async function loadEngine() {
  ready=false; updateButton(); $('evaluate-label').textContent='Loading engine'; $('retry-engine').hidden=true;
  $('engine-text').textContent='Loading WebAssembly…'; $('engine-status').firstElementChild.className='status-dot loading';
  try {
    engine = await import('./engine.js');
    const metadata = await engine.initEngine(); ready=true;
    $('engine-status').title = `${metadata.engine || 'Speedle+'} · ${metadata.revision?.slice(0,7) || ''} · ${metadata.runtime || 'WebAssembly'}`;
    if(engineErrorVisible) resetResult();
    $('engine-text').textContent='WebAssembly · Ready'; $('engine-status').firstElementChild.className='status-dot';
  } catch(error) {
    $('engine-text').textContent='Load failed · Retry'; $('engine-status').firstElementChild.className='status-dot error';
    $('retry-engine').hidden=false;
    showError({stage:'engine',message:`Unable to load the Speedle engine. ${error.message || String(error)}`});
  }
  updateButton();
}
async function evaluate() {
  if (!ready || busy) return;
  const policies=$('policies').value, version=inputVersion;
  resetResult();
  if (!policies.trim()) {showError({stage:'policies',message:'Enter at least one SPDL policy.'});$('policies').focus();return;}
  const converted=serializeRequest(readForm());
  if(!converted.ok){showFormErrors(converted.errors);showError({stage:'request',message:'Check the highlighted fields, then evaluate again.'});return;}
  const request=converted.request;
  busy=true; updateButton();
  $('result-card').className='result-card pending';icon('terminal');$('result-label').textContent='Evaluating…';$('result-description').textContent='Evaluating these policies and this request with Speedle.';$('result-meta').textContent='RUNNING';
  try {
    await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
    const result=await engine.evaluate({policies,request});
    if(version!==inputVersion) return;
    if(result.ok===false || result.error) {showError(result.error || {message:'Speedle did not return a valid result.'}, result);return;}
    if(typeof result.allowed!=='boolean') {showError({stage:'evaluation',message:'The engine did not return a valid boolean authorization result.'},result);return;}
    $('result-card').className=`result-card ${result.allowed?'allowed':'denied'}`;
    icon(result.allowed?'check':'close');$('result-label').textContent=String(result.allowed);
    const reasons={GRANT_POLICY_FOUND:'A grant policy matched. This request is allowed.',DENY_POLICY_FOUND:'An explicit deny policy matched. This request is denied.',NO_APPLICABLE_POLICIES:'No applicable grant policy. This request is denied by default.',SERVICE_NOT_FOUND:'Service not found. This request is denied.'};
    const codeReasons={0:reasons.GRANT_POLICY_FOUND,1:reasons.DENY_POLICY_FOUND,2:reasons.SERVICE_NOT_FOUND,3:reasons.NO_APPLICABLE_POLICIES};
    $('result-description').textContent=reasons[result.reason] || codeReasons[result.reasonCode] || (result.allowed?'This request is allowed.':'This request is not allowed.');
    $('result-type').textContent=result.allowed?'ACCESS GRANTED':'ACCESS DENIED';
    $('result-meta').textContent=`Speedle+ · ${typeof result.durationMs==='number'?`${result.durationMs.toFixed(2)} ms`:'Evaluation complete'}`;
    showRaw(result);
  } catch(error) {
    ready=false;
    $('engine-text').textContent='Reload required'; $('engine-status').firstElementChild.className='status-dot error'; $('retry-engine').hidden=false;
    if(version===inputVersion)showError({stage:'engine',message:error.message || String(error)});
  }
  finally {busy=false;updateButton();}
}
async function copyInput(id) {
  try {await navigator.clipboard.writeText($(id).value);notify('Copied to clipboard');}
  catch {$(id).focus();$(id).select();notify('Clipboard unavailable. Text selected; copy it manually.');}
}
document.querySelectorAll('[data-sample]').forEach(button=>button.addEventListener('click',()=>setSample(button.dataset.sample)));
$('evaluate').addEventListener('click',evaluate);
$('retry-engine').addEventListener('click',loadEngine);
$('reset-button').addEventListener('click',()=>{setSample('allow');notify('Default example restored');});
$('copy-policies').addEventListener('click',()=>copyInput('policies'));

document.addEventListener('keydown',event=>{if((event.ctrlKey||event.metaKey)&&event.key==='Enter'){event.preventDefault();evaluate();}});
if(/Mac|iPhone|iPad/.test(navigator.platform))$('shortcut-key').textContent='⌘';
let formState = requestToForm(baseRequest);
function element(tag, attrs = {}, text = '') {
  const node=document.createElement(tag);
  for(const [key,value] of Object.entries(attrs)) {
    if(key==='class')node.className=value;
    else if(key==='checked')node.checked=Boolean(value);
    else if(key==='value')node.value=value;
    else node.setAttribute(key,String(value));
  }
  if(text)node.textContent=text;
  return node;
}
function field(path,label,caption,value,kind='text',options) {
  const wrap=element('div',{'class':'form-field','data-field-wrapper':path});
  const id=`field-${path.replaceAll('.','-')}`;
  const title=element('label',{for:id});title.append(document.createTextNode(label));
  if(caption)title.append(element('span',{},caption));
  const control=options?element('select',{id,'data-field':path}):element('input',{id,'data-field':path,type:kind,autocomplete:'off'});
  if(options)for(const [v,t] of options)control.append(element('option',{value:v},t));
  else {control.maxLength=10000;if(kind==='number')control.step='any';if(kind==='datetime-local')control.step='1';}
  control.value=value ?? '';
  const message=element('small',{'class':'field-error',id:`${id}-error`,hidden:''});
  wrap.append(title,control,message);return wrap;
}
const principalTypes=[['user','user'],['group','group'],['entity','entity']];
const attributeTypes=[['string','string · Text'],['numeric','numeric · Number'],['bool','bool · Boolean'],['datetime','datetime · Date & time']];
function removeButton(label,action,index,valueIndex) {
  const button=element('button',{type:'button','class':'remove-button','aria-label':label,'data-action':action,'data-index':index,title:label},'×');
  if(valueIndex!==undefined)button.dataset.valueIndex=valueIndex;
  return button;
}
function renderLists(focusPath) {
  const principals=$('principal-list');principals.replaceChildren();
  formState.principals.forEach((principal,i)=>{
    const row=element('div',{'class':'principal-row','data-principal-index':i});
    row.append(field(`principals.${i}.type`,'Type','',principal.type,'text',principalTypes),field(`principals.${i}.name`,'Name','Required',principal.name),field(`principals.${i}.idd`,'Identity domain','Optional',principal.idd),removeButton(`Remove principal ${i+1}`,'remove-principal',i));
    principals.append(row);
  });
  if(!formState.principals.length)principals.append(element('p',{'class':'empty-state'},'No principals. Test an anonymous request, or add a user, group or entity.'));
  const attributes=$('attribute-list');attributes.replaceChildren();
  formState.attributes.forEach((attribute,i)=>{
    const row=element('div',{'class':'attribute-row','data-attribute-index':i});
    const head=element('div',{'class':'attribute-head'});
    head.append(field(`attributes.${i}.name`,'Name','Required',attribute.name),field(`attributes.${i}.type`,'Type','',attribute.type,'text',attributeTypes),removeButton(`Remove attribute ${i+1}`,'remove-attribute',i));
    const mode=element('label',{'class':'array-toggle'});const checkbox=element('input',{type:'checkbox','data-multiple':i,checked:attribute.multiple});
    mode.append(checkbox,document.createTextNode('Multiple values'),element('span',{},'Array'));
    const values=element('div',{'class':'attribute-values'});
    attribute.values.forEach((value,j)=>{
      const valueRow=element('div',{'class':'attribute-value'});
      const kind=attribute.type==='numeric'?'number':attribute.type==='datetime'?'datetime-local':'text';
      const valueField=field(`attributes.${i}.values.${j}`,attribute.multiple?`Value ${j+1}`:'Value',attribute.type==='datetime'?'UTC date & time':'',value,kind,attribute.type==='bool'?[['true','true'],['false','false']]:undefined);
      valueField.querySelector('input,select').dataset.attributeValue='';
      valueRow.append(valueField);
      if(attribute.multiple)valueRow.append(removeButton(`Remove value ${j+1} from attribute ${i+1}`,'remove-value',i,j));
      values.append(valueRow);
    });
    if(attribute.multiple && !attribute.values.length)values.append(element('p',{'class':'empty-state compact'},'Empty array. Select Add value to enter items.'));
    row.append(head,mode,values);
    if(attribute.multiple)row.append(element('button',{type:'button','class':'add-button add-value','data-action':'add-value','data-index':i},'+ Add value'));
    attributes.append(row);
  });
  if(!formState.attributes.length)attributes.append(element('p',{'class':'empty-state'},'No attributes. Add them when a policy condition needs them.'));
  if(focusPath){const control=$('request-form').querySelector(`[data-field="${focusPath}"]`);control?.focus();}
}
function populateForm(state) {
  formState=state;
  $('resource').value=state.resource;$('action').value=state.action;$('service-name').value=state.serviceName;
  renderLists();
}
function readForm() {
  const value=path=>$('request-form').querySelector(`[data-field="${path}"]`)?.value || '';
  return {
    resource:$('resource').value,action:$('action').value,serviceName:$('service-name').value,
    principals:[...$('principal-list').querySelectorAll('[data-principal-index]')].map((row,i)=>({type:value(`principals.${i}.type`),name:value(`principals.${i}.name`),idd:value(`principals.${i}.idd`)})),
    attributes:[...$('attribute-list').querySelectorAll('[data-attribute-index]')].map((row,i)=>({name:value(`attributes.${i}.name`),type:value(`attributes.${i}.type`),multiple:row.querySelector('[data-multiple]').checked,values:[...row.querySelectorAll('[data-attribute-value]')].map(control=>control.value)}))
  };
}
function clearFormErrors() {
  $('request-form').querySelectorAll('[aria-invalid]').forEach(control=>{control.removeAttribute('aria-invalid');control.removeAttribute('aria-describedby');});
  $('request-form').querySelectorAll('.field-error').forEach(message=>{message.hidden=true;message.textContent='';});
}
function showFormErrors(errors) {
  clearFormErrors();let first;
  for(const {path,message} of errors){
    const control=$('request-form').querySelector(`[data-field="${path}"]`);
    if(!control)continue;
    const box=control.closest('.form-field').querySelector('.field-error');
    if(!box.id)box.id=`error-${path.replaceAll('.','-')}`;
    box.textContent=message;box.hidden=false;control.setAttribute('aria-invalid','true');control.setAttribute('aria-describedby',box.id);first??=control;
  }
  first?.focus();
}
$('request-form').addEventListener('submit',event=>{event.preventDefault();evaluate();});
$('request-form').addEventListener('input',event=>{if(event.target.matches('[data-field]')){formState=readForm();changed();}});
$('request-form').addEventListener('change',event=>{
  formState=readForm();const target=event.target;
  if(target.matches('[data-multiple]')){
    const i=Number(target.dataset.multiple),attribute=formState.attributes[i];
    if(!attribute.multiple)attribute.values=[attribute.values[0] ?? (attribute.type==='bool'?'true':'')];
    changed();renderLists();
    $('attribute-list').querySelector(`[data-multiple="${i}"]`)?.focus();return;
  }
  if(target.dataset.field?.match(/^attributes\.\d+\.type$/)){
    const i=Number(target.dataset.field.split('.')[1]),attribute=formState.attributes[i];
    attribute.values=attribute.values.map(value=>attribute.type==='bool'?(value==='false'?'false':'true'):attribute.type==='datetime'?'':value);
    changed();renderLists(`attributes.${i}.type`);return;
  }
  changed();
});
$('request-form').addEventListener('click',event=>{
  const button=event.target.closest('button[data-action]');if(!button)return;
  formState=readForm();const i=Number(button.dataset.index),action=button.dataset.action;let focus;
  if(action==='remove-principal')formState.principals.splice(i,1);
  if(action==='remove-attribute')formState.attributes.splice(i,1);
  if(action==='remove-value')formState.attributes[i].values.splice(Number(button.dataset.valueIndex),1);
  if(action==='add-value'){const a=formState.attributes[i];a.values.push(a.type==='bool'?'true':'');focus=`attributes.${i}.values.${a.values.length-1}`;}
  changed();renderLists(focus);
  if(!focus){if(action==='remove-principal')$('add-principal').focus();else if(action==='remove-attribute')$('add-attribute').focus();else $('attribute-list').querySelector(`[data-action="add-value"][data-index="${i}"]`)?.focus();}
});
$('add-principal').addEventListener('click',()=>{formState=readForm();if(formState.principals.length>=200){notify('A maximum of 200 principals is supported');return;}formState.principals.push({type:'user',name:'',idd:''});changed();renderLists(`principals.${formState.principals.length-1}.name`);});
$('add-attribute').addEventListener('click',()=>{formState=readForm();formState.attributes.push({name:'',type:'string',multiple:false,values:['']});changed();renderLists(`attributes.${formState.attributes.length-1}.name`);});
setSample('allow');
loadEngine();
