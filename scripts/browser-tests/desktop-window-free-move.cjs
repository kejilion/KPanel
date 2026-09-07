const assert = require('node:assert/strict')
const fs = require('node:fs')
const os = require('node:os')
const { execFileSync } = require('node:child_process')
const { chromium } = require(process.env.KPANEL_PLAYWRIGHT_MODULE || 'playwright')
const root = process.cwd()
const base = new URL(process.env.COMBINED_PREVIEW).origin
assert.equal(new URL(base).hostname, '127.0.0.1')
const out = process.env.EVIDENCE_DIR
const candidate = execFileSync('git', ['rev-parse', 'HEAD'], {encoding:'utf8'}).trim()
assert.equal(candidate, process.env.REPORT_CANDIDATE)
assert.equal(execFileSync('git', ['status', '--porcelain'], {encoding:'utf8'}).trim(), '')
fs.mkdirSync(out, {recursive:true})
const report = {candidate, mode:'mock-ui', cases:[], errors:[], cleanup:false}
const close = (actual, expected) => assert(Math.abs(actual-expected)<2, `${actual} != ${expected}`)
;(async()=>{
 const browser = await chromium.launch({headless:true, executablePath:process.env.KPANEL_BROWSER_EXECUTABLE || undefined})
 report.browser = browser.version()
 let resourceFailure = false
 const watchdog = setInterval(()=>{ const disk=fs.statfsSync(root); if(os.freemem()<512*1024**2 || disk.bavail*disk.bsize<1024**3){resourceFailure=true; void browser.close()} },1000)
 try {
  for (const [width,height,theme,locale] of [[1920,1080,'dark','zh-CN'],[1280,800,'light','en-US'],[768,800,'dark','zh-CN'],[390,844,'light','zh-CN'],[640,450,'dark','en-US'],[1024,720,'light','en-US']]) {
   const context=await browser.newContext({viewport:{width,height},reducedMotion:'reduce',deviceScaleFactor:width===640?2:width===1024?1.25:1})
   await context.addInitScript(({theme,locale})=>{
    if(!localStorage.getItem('free-move-fixture')){
     localStorage.setItem('kejilion-panel-theme',theme)
     localStorage.setItem('kejilion-panel-locale',locale)
     localStorage.setItem('kejilion-panel-desktop-mode','desktop')
     localStorage.setItem('kejilion-panel-desktop-windows',JSON.stringify([{id:1,path:'/overview',geometry:{left:200,top:80,width:880,height:600},minimized:false,maximized:false,snap:null}]))
     localStorage.setItem('free-move-fixture','1')
    }
   },{theme,locale})
   const page=await context.newPage()
   page.on('pageerror',e=>report.errors.push(e.message))
   page.on('console',m=>{if(m.type()==='error')report.errors.push(m.text())})
   page.setDefaultTimeout(15000)
   await context.tracing.start({screenshots:true,snapshots:true})
   try {
    await page.goto(base + '/overview')
    const win=page.locator('.desktop-window').first()
    await win.waitFor({state:'visible'})
    await page.waitForFunction(()=>document.querySelector('.desktop-window')?.classList.contains('desktop-window--open'))
    const rect=async()=>{await win.evaluate(async el=>{await Promise.all(el.getAnimations().map(a=>a.finished.catch(()=>{})))});return win.boundingBox()}
    const state=async()=>page.evaluate(()=>JSON.parse(localStorage.getItem('kejilion-panel-desktop-windows'))[0])
    const drag=async(sx,sy,x,y)=>{await page.mouse.move(sx,sy);await page.mouse.down();await page.mouse.move(x,y,{steps:12});await page.mouse.up();await page.evaluate(()=>new Promise(requestAnimationFrame))}
    if(width<=760){
     const before=await rect()
     await drag(180,28,280,150)
     const after=await rect()
     close(after.x,before.x);close(after.y,before.y)
     report.cases.push({width,height,theme,locale,compactDragStable:true,zoomEmulation:width===640?'1280x900 at 200% effective CSS viewport':undefined})
    }else{
     let before=await rect()
     const targetX=width-230,targetY=Math.floor(height*.52)
     await drag(before.x+50,before.y+20,targetX+50,targetY+20)
     let moved=await rect()
     close(moved.x,targetX);close(moved.y,targetY);close(moved.width,before.width);close(moved.height,before.height)
     assert(moved.x+moved.width>width);assert(moved.y+moved.height>height)
     assert.equal((await state()).snap,null)
     assert.equal(await page.locator('.desktop-window-snap-preview').count(),0)
     await page.screenshot({path:`${out}/${width}-${theme}-partial.png`})
     await page.reload();await win.waitFor({state:'visible'});await page.waitForFunction(()=>document.querySelector('.desktop-window')?.classList.contains('desktop-window--open'))
     close((await rect()).x,moved.x);close((await rect()).y,moved.y)
     await drag(moved.x+50,moved.y+20,250,100)
     moved=await rect();close(moved.x,200);close(moved.y,80)
     // Grab near the middle to move the left window edge offscreen before the pointer reaches the snap zone.
     const grip=Math.min(400,moved.width-160)
     await drag(moved.x+grip,moved.y+20,80,220)
     moved=await rect();assert(moved.x<0);assert.equal((await state()).snap,null)
     await page.screenshot({path:`${out}/${width}-${theme}-left.png`})
     // Recover from the visible title-bar slice, then verify both side snaps and maximize.
     await drag(90,moved.y+20,500,160)
     for(const [x,y,expected] of [[8,300,'left'],[width-8,300,'right'],[Math.floor(width/2),8,'maximize']]){
      moved=await rect()
      const sx=Math.max(40,Math.min(width-170,moved.x+100))
      await page.mouse.move(sx,moved.y+20);await page.mouse.down();await page.mouse.move(x,y,{steps:15})
      await page.locator('.desktop-window-snap-preview').waitFor({state:'visible'})
      await page.mouse.up();await page.evaluate(()=>new Promise(requestAnimationFrame))
      const snap=await state();assert.equal(expected==='maximize'?snap.maximized:snap.snap,expected==='maximize'?true:expected)
      moved=await rect();console.log(JSON.stringify({width,expected,moved,scroll:await page.evaluate(()=>[scrollX,scrollY])}));assert(moved.x>=0&&moved.x+moved.width<=width+1);assert(moved.y+moved.height<=height-60)
      await drag(moved.x+Math.min(100,moved.width/2),moved.y+20,Math.floor(width/2),180)
      assert.equal((await state()).snap,null);assert.equal((await state()).maximized,false)
     }
     // Keyboard focus is still available after recovering the floating window.
     await win.focus();await page.keyboard.press('Tab')
     assert(await win.evaluate(el=>el.contains(document.activeElement)))
     const titleFont=await page.locator('.desktop-window__title').first().evaluate(el=>getComputedStyle(el).fontSize)
     report.cases.push({width,height,theme,locale,partialRightBottom:true,partialLeft:true,persistReload:true,dragBack:true,snapLeftRightMaximize:true,keyboardFocus:true,titleFont})
    }
    await context.tracing.stop({path:`${out}/${width}-trace.zip`})
   }finally{await context.close()}
  }
  assert.equal(resourceFailure,false)
  assert.deepEqual(report.errors,[])
 }catch(e){report.failure=e.stack;throw e}
 finally{clearInterval(watchdog);await browser.close();report.cleanup=true;fs.writeFileSync(`${out}/result.json`,JSON.stringify(report,null,2))}
 console.log(JSON.stringify(report))
})().catch(e=>{console.error(e);process.exitCode=1})
