/* Execute the workflow's actual GitHub scripts with isolated GitHub API mocks. */
'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');
const { test } = require('node:test');
const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;
const workflow = fs.readFileSync(path.resolve(__dirname, '../../.github/workflows/reviewbot.yml'), 'utf8');

function scriptFor(stepName) {
  const start = workflow.indexOf(`      - name: ${stepName}\n`);
  assert.notEqual(start, -1, `workflow step exists: ${stepName}`);
  const scriptStart = workflow.indexOf('          script: |\n', start);
  assert.notEqual(scriptStart, -1, `GitHub script exists: ${stepName}`);
  const lines = workflow.slice(scriptStart + '          script: |\n'.length).split('\n');
  const script = [];
  for (const line of lines) {
    if (line && !line.startsWith('            ')) break;
    script.push(line.slice(12));
  }
  return script.join('\n');
}
const mergeScript = scriptFor('Wait for required CI checks and merge');
const duplicateScript = scriptFor('Check for an existing review of this commit');
const publishScript = scriptFor('Create ReviewBot review for commit');
const HEAD = '1'.repeat(40), BASE = '2'.repeat(40), OTHER = '3'.repeat(40);
const owner = 'teramoby', repo = 'speedle-plus', number = 70;
const names = ['lint', 'test (file)', 'test (etcd)', 'build'];
const clone = (value) => JSON.parse(JSON.stringify(value));
function pull(overrides = {}) {
  return {state:'open', draft:false, user:{login:'dependabot[bot]'}, head:{sha:HEAD,repo:{full_name:`${owner}/${repo}`}},base:{sha:BASE},merge_commit_sha:OTHER,...overrides};
}
function run(overrides = {}) {
  return {id:101,head_sha:HEAD,event:'pull_request',path:'.github/workflows/ci.yml',head_repository:{full_name:`${owner}/${repo}`},pull_requests:[{number,head:{sha:HEAD},base:{sha:BASE}}],status:'completed',conclusion:'success',...overrides};
}
function jobs() { return names.map((name,i)=>({id:i+1,name,head_sha:HEAD,status:'completed',conclusion:'success'})); }
function after(sequence, index) { return clone(sequence[Math.min(index, sequence.length-1)]); }

async function executeMerge(options = {}) {
  const calls = [], failures = [], notices = [], outputs = {};
  let now = 0, pulls = 0, runs = 0, jobReads = 0, merged = false;
  const context = {repo:{owner,repo},issue:{number},payload:{pull_request:pull(options.eventPull)}};
  const rest = {pulls:{},actions:{}};
  rest.pulls.get = async (args) => {
    calls.push(['pull',args]);
    if(options.getError)throw options.getError;
    return {data:after(options.pulls || [pull()], pulls++)};
  };
  rest.actions.listWorkflowRuns = Symbol('listWorkflowRuns');
  rest.actions.listJobsForWorkflowRun = Symbol('listJobsForWorkflowRun');
  rest.pulls.merge = async (args) => {
    calls.push(['merge',args]);
    if(options.mergeError)throw options.mergeError;
    const data = options.mergeResult || {merged:true};
    merged = data.merged;
    return {data};
  };
  const github = {rest,paginate:async(method,args)=>{
    if(method === rest.actions.listWorkflowRuns){
      calls.push(['runs',args]);
      if(options.runsError)throw options.runsError;
      return after(options.runs || [[run()]], runs++);
    }
    if(method === rest.actions.listJobsForWorkflowRun){
      calls.push(['jobs',args]);
      if(options.jobsError)throw options.jobsError;
      return after(options.jobs || [jobs()], jobReads++);
    }
    throw new Error('unexpected pagination endpoint');
  }};
  const core = {setFailed:message=>failures.push(message),notice:message=>notices.push(message),setOutput:(name,value)=>outputs[name]=value};
  const DateMock = {now:()=>now};
  const setTimeoutMock = (resolve,ms) => {calls.push(['wait',ms]);now += options.waitAdvance || ms;resolve();};
  let thrown;
  try {await new AsyncFunction('github','context','core','Date','setTimeout',mergeScript)(github,context,core,DateMock,setTimeoutMock);}
  catch(error){thrown=error;}
  return {calls,failures,notices,outputs,merged,thrown};
}
function didNotMerge(result) {
  assert.equal(result.calls.some(([type])=>type==='merge'), false, JSON.stringify(result));
  assert.equal(result.merged,false);
}
function failed(result, pattern) {
  didNotMerge(result);
  assert.ok(result.thrown || result.failures.length, JSON.stringify(result));
  if(pattern) assert.match(result.thrown?.message || result.failures.join('\n'),pattern);
}
function passed(result) {
  assert.equal(result.thrown,undefined);
  assert.deepEqual(result.failures,[]);
  assert.equal(result.merged,true);
  assert.deepEqual(result.calls.filter(([type])=>type==='merge').map(([,args])=>args),[{owner,repo,pull_number:number,merge_method:'squash',sha:HEAD}]);
}

test('successful gate queries ci.yml pull_request HEAD, latest jobs, and exact squash SHA', async()=>{
  const result=await executeMerge();passed(result);
  assert.deepEqual(result.calls.find(([type])=>type==='runs')[1],{owner,repo,workflow_id:'ci.yml',event:'pull_request',head_sha:HEAD,per_page:100});
  assert.deepEqual(result.calls.find(([type])=>type==='jobs')[1],{owner,repo,run_id:101,filter:'latest',per_page:100});
  assert.equal(JSON.stringify(result.calls).includes(OTHER),false,'synthetic merge SHA must not be used for checks');
});
test('job-level condition requires safe verdict and same-repository Dependabot',()=>{
  const job=workflow.slice(workflow.indexOf('  automerge-dependabot:'));
  assert.match(job,/github\.event\.pull_request\.user\.login == 'dependabot\[bot\]'/);
  assert.match(job,/github\.event\.pull_request\.head\.repo\.full_name == github\.repository/);
  assert.match(job,/needs\.review\.outputs\.safe_to_merge == 'true'/);
  assert.match(job,/needs: \[review, publish\]/);
});
for(const [name,eventPull] of [['human author',{user:{login:'maintainer'}}],['fork head',{head:{sha:HEAD,repo:{full_name:'other/speedle-plus'}}}]])test(`event gate rejects ${name}`,async()=>{
  const result=await executeMerge({eventPull});failed(result,/Only same-repository Dependabot/);assert.equal(result.calls.length,0);
});
for(const [name,changed] of [['closed',{state:'closed'}],['draft',{draft:true}],['changed head',{head:{sha:OTHER}}],['changed base',{base:{sha:OTHER}}]])test(`live pull gate rejects ${name}`,async()=>{
  const result=await executeMerge({pulls:[pull(changed)]});failed(result,/state, head or base changed/);assert.equal(result.calls.some(([type])=>type==='runs'),false);
});
for(const conclusion of ['failure','cancelled','skipped','timed_out','neutral','action_required'])test(`CI run ${conclusion} does not authorize merge`,async()=>failed(await executeMerge({runs:[[run({conclusion})]]}),/CI did not pass/));
for(const conclusion of ['failure','cancelled','skipped','timed_out','neutral','action_required'])test(`required job ${conclusion} does not authorize merge`,async()=>{
  const list=jobs();list[0].conclusion=conclusion;failed(await executeMerge({jobs:[list]}),/Required CI did not pass/);
});
for(const [name,list] of [['missing job',jobs().slice(1)],['wrong head job',jobs().map(j=>({...j,head_sha:OTHER}))],['pending job',jobs().map(j=>({...j,status:'in_progress',conclusion:null}))]])test(`${name} waits then times out without merge`,async()=>{
  const result=await executeMerge({jobs:[list],waitAdvance:900000});failed(result,/Timed out/);assert.equal(result.calls.filter(([type])=>type==='wait').length,1);
});
for(const [name,overrides] of [['wrong workflow',{path:'.github/workflows/other.yml'}],['wrong event',{event:'push'}],['wrong PR',{pull_requests:[{number:71,head:{sha:HEAD},base:{sha:BASE}}]}],['wrong head repository',{head_repository:{full_name:'other/speedle-plus'}}],['missing head repository',{head_repository:null}],['wrong head',{head_sha:OTHER}],['old base CI',{pull_requests:[{number,head:{sha:HEAD},base:{sha:OTHER}}]}],['wrong PR metadata head',{pull_requests:[{number,head:{sha:OTHER},base:{sha:BASE}}]}]])test(`${name} run cannot satisfy trusted CI`,async()=>{
  const result=await executeMerge({runs:[[run(overrides)]],waitAdvance:900000});failed(result,/Timed out/);assert.equal(result.calls.some(([type])=>type==='jobs'),false);
});
test('newest failed run supersedes older successful run even when listed out of order',async()=>failed(await executeMerge({runs:[[run({id:101}),run({id:102,conclusion:'failure'})]]}),/CI did not pass/));
test('latest successful run selected instead of older failure',async()=>{
  const result=await executeMerge({runs:[[run({id:100,conclusion:'failure'}),run({id:102})]]});passed(result);assert.equal(result.calls.find(([type])=>type==='jobs')[1].run_id,102);
});
test('newest pending run supersedes older success and then completes',async()=>{
  const result=await executeMerge({runs:[[run(),run({id:102,status:'in_progress',conclusion:null})],[run(),run({id:102})]]});passed(result);assert.equal(result.calls.filter(([type])=>type==='wait').length,1);assert.equal(result.calls.find(([type])=>type==='jobs')[1].run_id,102);
});
test('missing CI run can arrive after polling',async()=>passed(await executeMerge({runs:[[],[run()]]})));
test('pending required jobs can complete after polling',async()=>passed(await executeMerge({jobs:[jobs().map(j=>({...j,status:'in_progress',conclusion:null})),jobs()]})));
test('changed head during a CI wait prevents a later merge',async()=>failed(await executeMerge({runs:[[],[run()]],pulls:[pull(),pull({head:{sha:OTHER}})]}),/state, head or base changed/));
test('changed base during a CI wait prevents a later merge',async()=>failed(await executeMerge({runs:[[],[run()]],pulls:[pull(),pull({base:{sha:OTHER}})]}),/state, head or base changed/));
for(const name of ['getError','runsError','jobsError','mergeError'])test(`${name} fails closed`,async()=>{
  const result=await executeMerge({[name]:new Error(name)});assert.match(result.thrown?.message || '',new RegExp(name));assert.equal(result.merged,false);
});
test('merge API merged=false is reported as failure',async()=>{
  const result=await executeMerge({mergeResult:{merged:false,message:'base branch modified'}});assert.equal(result.merged,false);assert.match(result.failures.join('\n'),/base branch modified/);
});

async function executeReview(script,reviews,env = {}) {
  const outputs={},created=[],notices=[];
  const listReviews=Symbol('listReviews');
  const github={rest:{pulls:{listReviews,createReview:async(args)=>{created.push(args);return {data:{id:9}};}}},paginate:async(method,args)=>{assert.equal(method,listReviews);assert.equal(args.pull_number,number);return clone(reviews);}};
  const context={repo:{owner,repo},issue:{number}};
  const core={setOutput:(name,value)=>outputs[name]=value,notice:message=>notices.push(message)};
  const processMock={env:{REVIEWED_COMMIT:HEAD,REVIEWBOT_MESSAGE:'No actionable findings.',MODEL_ID_B64:Buffer.from('kimi-k3').toString('base64'),...env}};
  await new AsyncFunction('github','context','core','process','Buffer',script)(github,context,core,processMock,Buffer);
  return {outputs,created,notices};
}
const marker=`<!-- speedle-plus-reviewbot:${HEAD} -->`;
function review(overrides={}) { return {id:8,user:{login:'github-actions[bot]'},commit_id:HEAD,state:'COMMENTED',body:marker+'\nDetailed review.',...overrides}; }
test('same-commit bot review deduplicates before model call',async()=>assert.equal((await executeReview(duplicateScript,[review()])).outputs.should_review,'false'));
test('same-commit bot review deduplicates before publish',async()=>assert.equal((await executeReview(publishScript,[review()])).created.length,0));
test('human copying a marker cannot suppress analysis',async()=>assert.equal((await executeReview(duplicateScript,[review({user:{login:'maintainer'}})])).outputs.should_review,'true'));
test('old commit marker cannot suppress current commit analysis',async()=>assert.equal((await executeReview(duplicateScript,[review({commit_id:OTHER,body:`<!-- speedle-plus-reviewbot:${OTHER} -->`})])).outputs.should_review,'true'));
test('new formal review is COMMENT bound to reviewed commit',async()=>{
  const result=await executeReview(publishScript,[]);assert.equal(result.created.length,1);assert.equal(result.created[0].event,'COMMENT');assert.equal(result.created[0].commit_id,HEAD);assert.ok(result.created[0].body.startsWith(marker+'\n'));
});
for(const [name,overrides] of [['marker quoted in another review',{body:'Quoted old output:\n'+marker}],['marker on a different commit',{commit_id:OTHER}]]) {
  test(`${name} cannot suppress analysis`,async()=>assert.equal((await executeReview(duplicateScript,[review(overrides)])).outputs.should_review,'true'));
  test(`${name} cannot suppress publication`,async()=>assert.equal((await executeReview(publishScript,[review(overrides)])).created.length,1));
}

test('long review publication preserves exact marker, commit and model within the limit',async()=>{
  const result=await executeReview(publishScript,[],{REVIEWBOT_MESSAGE:'x'.repeat(70000),MODEL_ID_B64:Buffer.from('model-'+ 'm'.repeat(2000)).toString('base64')});
  assert.equal(result.created.length,1);
  const body=result.created[0].body;
  assert.ok(body.length<=65000);
  assert.ok(body.startsWith(marker+'\n'));
  assert.ok(body.includes(`_Reviewed commit: \`${HEAD}\`_`));
  assert.ok(body.includes('_ReviewBot output was truncated'));
  assert.ok(body.endsWith('m'.repeat(2000)+'`_'));
});

const eligibilityStart = workflow.indexOf('          # Preserve the existing dependency-only merge contract conservatively.\n');
const eligibilityEnd = workflow.indexOf('          model_id_b64=',eligibilityStart);
assert.ok(eligibilityStart>=0 && eligibilityEnd>eligibilityStart,'actual safe-to-merge block must exist');
const eligibilityScript=workflow.slice(eligibilityStart,eligibilityEnd).split('\n').map(line=>line.slice(10)).join('\n');
const safeMarker='<!-- reviewbot-verdict:no-actionable-findings -->';
const actionableMarker='<!-- reviewbot-verdict:actionable-findings -->';
function safeToMerge({inputTruncated=false,attempt=1,message=safeMarker}={}) {
  const directory=fs.mkdtempSync(path.join(os.tmpdir(),'reviewbot-automerge-'));
  const output=path.join(directory,'output');
  try {
    const result=spawnSync('bash',['-c','set -euo pipefail\n'+eligibilityScript],{encoding:'utf8',env:{...process.env,input_truncated:String(inputTruncated),attempt:String(attempt),review_message:message,GITHUB_OUTPUT:output}});
    assert.equal(result.status,0,result.stderr);
    return fs.readFileSync(output,'utf8').trim();
  } finally {fs.rmSync(directory,{recursive:true,force:true});}
}
test('complete first-attempt exact no-findings verdict is eligible',()=>assert.equal(safeToMerge(),'safe_to_merge=true'));
test('truncated input review cannot authorize auto-merge',()=>assert.equal(safeToMerge({inputTruncated:true}),'safe_to_merge=false'));
test('reduced-context retry cannot authorize auto-merge',()=>assert.equal(safeToMerge({attempt:2}),'safe_to_merge=false'));
test('actionable verdict cannot authorize auto-merge',()=>assert.equal(safeToMerge({message:actionableMarker}),'safe_to_merge=false'));
test('contradictory markers cannot authorize auto-merge',()=>assert.equal(safeToMerge({message:safeMarker+'\n'+actionableMarker}),'safe_to_merge=false'));
test('missing verdict cannot authorize auto-merge',()=>assert.equal(safeToMerge({message:'No actionable findings.'}),'safe_to_merge=false'));
test('inexact verdict marker cannot authorize auto-merge',()=>assert.equal(safeToMerge({message:'Quoted: '+safeMarker}),'safe_to_merge=false'));
