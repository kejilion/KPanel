// Bounded metadata only: no headers, cookies, bodies, console text, input values or terminal output.
function createDiagnostics(context, base, { captureTimeoutMs = 2000 } = {}) {
 const started = Date.now(), pending = new Map(), attached = new WeakSet();
 const data = { stage: 'bootstrap', events: [], droppedEvents: 0, untrackedRequests: 0 };
 const route = value => {
  try {
   const url = new URL(value, base);
   if (url.origin !== new URL(base).origin) return '[external]';
   if (/^\/assets\//.test(url.pathname)) return '/assets/*';
   if (/^\/api\/v1\/terminal-sessions\/[^/]+(?:\/(input|output|resize|close))?$/.test(url.pathname)) {
    return url.pathname.replace(/(\/terminal-sessions\/)[^/]+/, '$1:id');
   }
   return /^(?:\/|\/terminal|\/login|\/bootstrap|\/settings|\/api\/v1\/(?:auth\/(?:bootstrap|login|session)|settings\/(?:preferences|kpanel-release|automatic-update(?:\/install)?)|cluster\/hosts|terminal-sessions|terminal-history))$/.test(url.pathname)
    ? url.pathname : '[other]';
  } catch { return '[invalid]'; }
 };
 const add = (kind, detail) => {
  if (data.events.length === 128) { data.events.shift(); data.droppedEvents++; }
  data.events.push({ ms: Date.now() - started, stage: data.stage, kind, ...detail });
 };
 const describe = request => ({ route: route(request.url()),
  method: /^(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)$/.test(request.method()) ? request.method() : '[other]' });
 const attach = page => {
  if (attached.has(page)) return;
  attached.add(page);
  page.on('pageerror', () => add('pageerror', { route: route(page.url()) }));
 };
 context.on('page', attach);
 for (const page of context.pages()) attach(page);
 context.on('request', request => {
  const detail = describe(request);
  if (pending.size < 32) pending.set(request, { ...detail, stage: data.stage, startedMs: Date.now() - started });
  else data.untrackedRequests++;
 });
 context.on('response', response => add('response', { ...describe(response.request()), status: response.status() }));
 context.on('requestfinished', request => pending.delete(request));
 context.on('requestfailed', request => {
  pending.delete(request);
  const code = /\bnet::ERR_[A-Z_]+\b/.exec(request.failure()?.errorText || '')?.[0] || 'request-failed';
  add('requestfailed', { ...describe(request), code });
 });
 async function snapshot(page, index) {
  let timer;
  try {
   const state = await Promise.race([
    page.evaluate(() => {
     // Counts/visibility distinguish login, bootstrap and missing terminal tabs without exporting DOM text.
     const selectors = ['input[type="password"]', '.terminal-tab__close', '.terminal-tab__status.is-connected',
      '.terminal-alert', '[role="alert"]', '#bootstrap-recovery', '[data-testid="preview-program-toggle"]'];
     return selectors.map(selector => {
      const elements = document.querySelectorAll(selector), count = Math.min(elements.length, 32);
      let visible = 0;
      for (let i = 0; i < count; i++) if (elements[i].getClientRects().length > 0) visible++;
      return { selector, count, visible, capped: elements.length > count };
     });
    }),
    new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('capture deadline')), captureTimeoutMs); }),
   ]);
   return { index, route: route(page.url()), status: 'captured', state };
  } catch { return { index, status: 'unavailable' }; }
  finally { clearTimeout(timer); }
 }
 return {
  data,
  stage(value) { data.stage = value; },
  api(path, method, status) { add('api-response', { route: route('/api/v1' + path), method, status }); },
  async apiRequest(path, method, send) {
   const key = {}, requestStarted = Date.now();
   const detail = { route: route('/api/v1' + path),
    method: /^(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)$/.test(method) ? method : '[other]' };
   add('api-request', detail);
   if (pending.size < 32) pending.set(key, { ...detail, stage: data.stage, startedMs: requestStarted - started });
   else data.untrackedRequests++;
   try {
    const response = await send();
    add('api-response', { ...detail, status: response.status(), durationMs: Math.max(0, Date.now() - requestStarted) });
    return response;
   } catch (error) {
    // Inspect locally for a fixed error class; never copy exception text or request options into diagnostics.
    const message = typeof error?.message === 'string' ? error.message : '';
    const code = error?.name === 'TimeoutError' || /\b[Tt]imeout\b.*\bexceeded\b/.test(message) ? 'timeout'
     : /\b(?:ECONNRESET|ECONNREFUSED|ECONNABORTED|ENOTFOUND|EAI_AGAIN|EPIPE)\b/.exec(message)?.[0] || 'transport-error';
    add('api-requestfailed', { ...detail, code, durationMs: Math.max(0, Date.now() - requestStarted) });
    throw error;
   } finally { pending.delete(key); }
  },
  async capture() {
   try {
    const pages = context.pages();
    return { stage: data.stage, omittedPages: Math.max(0, pages.length - 4), pending: [...pending.values()].map(entry => ({ ...entry,
     ageMs: Date.now() - started - entry.startedMs })),
     pages: await Promise.all(pages.slice(0, 4).map(snapshot)) };
   } catch { return { stage: data.stage, status: 'unavailable' }; }
  },
 };
}

module.exports = { createDiagnostics };
if (require.main === module) {
const {chromium}=require('/work/runtime/node_modules/playwright');
const fs=require('node:fs'),crypto=require('node:crypto'),assert=require('node:assert/strict');
const base='http://127.0.0.1:'+process.argv[2],mode=process.argv[3],digest=process.argv[4],out='/work/evidence';
const isPreview=process.env.EXPECTED_VERSION.includes('-rc.');
const report={mode,candidate:process.env.EXPECTED_SHA,startedAt:new Date().toISOString(),mocked:false,frontendVersion:process.env.EXPECTED_VERSION,targetDigest:digest,checks:[],errors:[],screenshots:[]};
const check=(n,v)=>{assert(v,n);report.checks.push(n)};const unwrap=x=>x&&typeof x.data==='object'&&x.data!==null&&!Array.isArray(x.data)?x.data:x;
(async()=>{const browser=await chromium.launch({headless:true,args:['--no-sandbox']});let diagnostics;try{
 const ctx=await browser.newContext({viewport:{width:1280,height:900}});diagnostics=createDiagnostics(ctx,base);report.diagnostics=diagnostics.data;await ctx.addInitScript(()=>localStorage.setItem('kejilion-panel-locale','zh-CN'));
 const boot=await diagnostics.apiRequest('/auth/bootstrap','POST',()=>ctx.request.post(base+'/api/v1/auth/bootstrap',{headers:{Origin:base},data:{token:fs.readFileSync('/work/bootstrap.token','utf8').trim(),username:'rc127acceptance',password:'Accept!'+crypto.randomBytes(32).toString('hex')}}));check('real isolated bootstrap',boot.status()===201);
 const session=unwrap(await boot.json()),headers={Origin:base,'X-CSRF-Token':session.csrfToken};
 async function api(path,method='GET',data){const r=await diagnostics.apiRequest(path,method,()=>ctx.request.fetch(base+'/api/v1'+path,{method,headers,data,timeout:90000}));check('real API '+method+' '+path,r.ok());return unwrap(await r.json())}
 if(mode==='published'){
  const opened=await api('/terminal-sessions','POST',{hostId:'local',rows:24,columns:80,retained:true});
  check('real isolated PTY retained acknowledgement',opened.retained===true&&typeof opened.sessionId==='string');report.terminalOpened=opened;
  const id=opened.sessionId;await api('/terminal-sessions/'+id+'/input','POST',{data:Buffer.from('stty echo\r').toString('base64').replace(/=+$/,'')});
  await api('/terminal-sessions/'+id+'/output?offset=0&wait=1000');
  await api('/terminal-sessions/'+id+'/input','POST',{data:Buffer.from("printf '%s%s\\n' '__KPANEL_RC127_' 'PTY__'\r").toString('base64').replace(/=+$/,'')});
  let output,transcript='',offset=0;for(let i=0;i<20;i++){output=await api('/terminal-sessions/'+id+'/output?offset='+offset+'&wait=1000');assert(output&&typeof output==='object'&&(typeof output.data==='string'||output.data===null)&&Number.isSafeInteger(output.nextOffset),'terminal output schema');transcript+=Buffer.from(output.data||'','base64').toString();offset=output.nextOffset;if(transcript.includes('__KPANEL_RC127_PTY__'))break}
  check('real PTY execution output marker',transcript.includes('__KPANEL_RC127_PTY__'));
  const terminalPage=await ctx.newPage();terminalPage.on('pageerror',()=>report.errors.push('pageerror'));diagnostics.stage('terminal-first-navigation');await terminalPage.goto(base+'/terminal');await terminalPage.locator('.terminal-tab__close').waitFor();diagnostics.stage('terminal-reload');await terminalPage.reload();await terminalPage.locator('.terminal-tab__close').waitFor();diagnostics.stage('terminal-history');
  const retained=await api('/terminal-sessions');check('same real session survives page reload',retained.items.some(x=>x.sessionId===id&&x.retained));
  await api('/terminal-sessions/'+id+'/resize','POST',{rows:30,columns:100});
  check('one restored terminal tab',await terminalPage.locator('.terminal-tab__close').count()===1);
  // Run through the real composer and shell echo, then read the real Panel store.
  await terminalPage.locator('.terminal-tab__status.is-connected').first().waitFor({state:'attached'});
  await terminalPage.screenshot({path:out+'/terminal-before-history.png',fullPage:true});report.screenshots.push('terminal-before-history.png');
  const composer=terminalPage.locator('.host-terminal__composer input').first();await composer.waitFor();
  const historyCommand='printf RC6_HISTORY_UI';
  const appended=terminalPage.waitForResponse(r=>r.url()===base+'/api/v1/terminal-history'&&r.request().method()==='POST');
  await composer.fill(historyCommand);await composer.press('Enter');check('real composer command captured',(await appended).status()===200);
  check('real history API contains executed command',(await api('/terminal-history')).items.some(x=>x.command===historyCommand&&x.hostId==='local'));
  await terminalPage.locator('[aria-controls="terminal-quick-commands"]:visible').click();await terminalPage.getByRole('tab',{name:/历史命令/}).click();
  await terminalPage.locator('.terminal-history-entry__fill').filter({hasText:historyCommand}).waitFor();
  check('history privacy notice visible',(await terminalPage.locator('.terminal-command-history__notice').innerText()).includes('Token'));
  await terminalPage.locator('.terminal-history-entry__fill').filter({hasText:historyCommand}).click();check('history fills composer without execution',await composer.inputValue()===historyCommand);
  await terminalPage.screenshot({path:out+'/terminal-history-real.png',fullPage:true});report.screenshots.push('terminal-history-real.png');
  await api('/terminal-history','DELETE');check('history clear API readback',(await api('/terminal-history')).items.length===0);
  await terminalPage.screenshot({path:out+'/terminal-restored.png',fullPage:true});report.screenshots.push('terminal-restored.png');
  const closeReply=terminalPage.waitForResponse(r=>r.url()===base+'/api/v1/terminal-sessions/'+id+'/close'&&r.request().method()==='POST');
  await terminalPage.getByRole('button',{name:'关闭终端',exact:true}).click();const closedResponse=await closeReply;const closed=unwrap(await closedResponse.json());check('real PTY termination confirmed through UI',closedResponse.status()===200&&closed.closed===true);
  check('closed real session removed',(await api('/terminal-sessions')).items.every(x=>x.sessionId!==id));
  const rejected=await diagnostics.apiRequest('/terminal-sessions/'+id+'/input','POST',()=>ctx.request.post(base+'/api/v1/terminal-sessions/'+id+'/input',{headers,data:{data:'eA'}}));check('input to closed terminal refused',rejected.status()===404);await terminalPage.close();
  report.terminalEvidence='Exact published Agent binary in an isolated bash runner; no host filesystem/Docker/systemd socket. Does not establish real mixed-version node compatibility or host systemd behavior.';
 }
 diagnostics.stage('release-info');const release=await api('/settings/kpanel-release?channel='+(isPreview?'preview':'stable'));report.release=release;check('official release version',release.version===process.env.EXPECTED_VERSION);check('official immutable digest',release.imageDigest===digest);check('nonempty real notes',release.notes?.length===Number(process.env.EXPECTED_NOTES));check('real upgrade notes',release.upgradeNotes?.length===Number(process.env.EXPECTED_UPGRADE_NOTES));check('official release URL',release.releaseUrl==='https://github.com/kejilion/KPanel/releases/tag/v'+process.env.EXPECTED_VERSION);
 const stable=await api('/settings/kpanel-release?channel=stable');check('stable channel expected version',stable.version===process.env.EXPECTED_STABLE_VERSION);
 diagnostics.stage('settings-navigation');const page=await ctx.newPage();page.on('pageerror',()=>report.errors.push('pageerror'));await page.goto(base+'/settings?section=version-updates');const toggle=page.getByTestId('preview-program-toggle');await toggle.waitFor();
 let state=await api('/settings/automatic-update');check('default stable with auto install off',state.channel==='stable'&&!state.enabled);
 page.once('dialog',d=>d.dismiss());await toggle.click();check('declined opt-in leaves stable',(await api('/settings/automatic-update')).channel==='stable');
 page.once('dialog',d=>d.accept());await toggle.click();await page.waitForFunction(()=>{const e=document.querySelector('[data-testid="preview-program-toggle"]');return e?.checked&&!e.disabled},null,{timeout:90000});state=await api('/settings/automatic-update');check('opt-in does not enable or request installation',state.channel==='preview'&&!state.enabled&&!state.installRequested);
 if(!isPreview){await toggle.click();await page.waitForFunction(()=>{const e=document.querySelector('[data-testid="preview-program-toggle"]');return e&&!e.checked&&!e.disabled},null,{timeout:90000});state=await api('/settings/automatic-update');check('stable release checked from default stable channel',state.channel==='stable'&&!state.enabled&&!state.installRequested)}
 await page.getByTestId('check-release-update').click();await page.waitForFunction(()=>!document.querySelector('[data-testid="check-release-update"]').disabled,null,{timeout:90000});state=await api('/settings/automatic-update');report.checked=state;check('real release check completes',!!state.lastCheckedAt&&!state.lastErrorCode);
 if(mode==='published'){check('new binary current version',state.currentVersion===process.env.EXPECTED_VERSION);check('same version not offered for reinstall',!state.canInstall&&!state.candidateVersion)}
 else{
  check('old backend discovers current preview',state.currentVersion===process.env.PREVIOUS_PREVIEW_VERSION&&state.candidateVersion===process.env.EXPECTED_VERSION&&state.candidateImageDigest===digest&&state.canInstall);
  await page.getByTestId('manual-release-update').click();const dialog=page.getByTestId('kpanel-update-dialog');await dialog.waitFor();check('dialog binds exact version',(await dialog.innerText()).includes('v'+process.env.EXPECTED_VERSION));check('dialog contains real notes',await dialog.locator('.kpanel-release-notes li').count()===Number(process.env.EXPECTED_NOTES));check('dialog respects three upgrade-note display limit',await dialog.locator('.kpanel-upgrade-notes li').count()===Number(process.env.EXPECTED_UPGRADE_NOTES));
  for(const width of [1280,390,320]){await page.setViewportSize({width,height:900});await page.screenshot({path:out+'/dialog-'+width+'.png',fullPage:true});report.screenshots.push('dialog-'+width+'.png');check('no horizontal overflow '+width,await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1))}
  const response=page.waitForResponse(r=>r.url()===base+'/api/v1/settings/automatic-update/install'&&r.request().method()==='POST');await page.getByRole('button',{name:'更新到 v'+process.env.EXPECTED_VERSION,exact:true}).click();const attempted=await response;report.installFailure={status:attempted.status(),body:await attempted.json()};check('real no-systemd dispatch fails closed',attempted.status()===503&&report.installFailure.body.code==='self_update_start_failed');
  await page.getByText('立即安装失败',{exact:true}).waitFor();state=await api('/settings/automatic-update');check('real failed dispatch clears queue',!state.installRequested&&!state.enabled&&state.state!=='queued'&&state.state!=='updating');await page.screenshot({path:out+'/failure-320.png',fullPage:true});report.screenshots.push('failure-320.png');
  await page.getByRole('button',{name:'取消',exact:true}).click();const switched=page.waitForResponse(r=>r.url()===base+'/api/v1/settings/automatic-update'&&r.request().method()==='PUT');if(!isPreview)page.once('dialog',d=>d.accept());await toggle.click();const r=await switched;check('new frontend switches after real 503 without reload',r.status()===200);state=await api('/settings/automatic-update');check('channel switch retains installation off',state.channel===(isPreview?'stable':'preview')&&!state.enabled&&!state.installRequested);
  if(!isPreview){await toggle.click();await page.waitForFunction(()=>{const e=document.querySelector('[data-testid="preview-program-toggle"]');return e&&!e.checked&&!e.disabled},null,{timeout:90000});state=await api('/settings/automatic-update');check('stable channel restored after failure',state.channel==='stable'&&!state.enabled&&!state.installRequested)}
 }
 if(mode==='published'){if(isPreview){await toggle.click();await page.waitForFunction(()=>{const e=document.querySelector('[data-testid="preview-program-toggle"]');return e&&!e.checked&&!e.disabled},null,{timeout:90000})}state=await api('/settings/automatic-update');check('exit does not downgrade',state.channel==='stable'&&!state.canInstall&&!state.installRequested&&state.currentVersion===process.env.EXPECTED_VERSION)}
 await page.setViewportSize({width:1280,height:900});await page.screenshot({path:out+'/final.png',fullPage:true});report.screenshots.push('final.png');check('page errors zero',report.errors.length===0);report.final=state;report.status='passed';
 }catch(e){report.status='failed';throw e}finally{if(report.status!=='passed'&&diagnostics)report.diagnostics.failure=await diagnostics.capture();try{await browser.close()}catch(e){if(report.status==='failed')report.browserCloseFailed=true;else throw e}}})().catch(e=>{report.status='failed';report.error='browser workflow failed';process.exitCode=1}).finally(()=>{report.finishedAt=new Date().toISOString();fs.writeFileSync(out+'/report.json',JSON.stringify(report,null,2));console.log(JSON.stringify({status:report.status,mode,checks:report.checks.length,error:report.error}))});
}
