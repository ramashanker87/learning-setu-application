// Local synthetic data only. Install playwright-core outside the repository, then
// set PLAYWRIGHT_MODULE to its absolute index.mjs path if it is not on the module path.
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || 'playwright-core');
import assert from 'node:assert/strict';
const browser=await chromium.launch({executablePath:process.env.CHROME_PATH || '/usr/bin/google-chrome',headless:true,args:['--no-sandbox']});
const page=await browser.newPage({viewport:{width:1440,height:1050}});
const errors=[];page.on('pageerror',e=>errors.push(e.message));
await page.goto('http://localhost:8080');
await page.getByRole('heading',{name:'Sign in to Learning Setu'}).waitFor();
await page.screenshot({path:'/tmp/setu-login-desktop.png',fullPage:true});
await page.locator('.apply-primary').click();
assert.equal(await page.locator('header .header-link').count(),0);
assert.equal(await page.getByRole('heading',{name:'Continue an application',exact:true}).count(),0);
await page.getByRole('button',{name:'Start application',exact:true}).click();
assert.equal(await page.locator('[name=pan], [name=firstName], [name=lastName]').count(),0);
await page.getByLabel('Contact Person Name', {exact:true}).waitFor();
for(const [name,value] of Object.entries({contactPersonName:'Test Representative',address:'Test Road',contactEmail:'alpha@example.test',contactPhone:'9999999999',gstin:'22ABCDE1234F1Z5',orgNo:'BROWSER-TEST'})) await page.locator(`[name="${name}"]`).fill(value);
await page.locator('[name=termsAccepted]').check();await page.locator('[name=authorityDeclared]').check();
await page.getByRole('button',{name:'Save application',exact:true}).click();
await page.getByRole('heading',{name:'Keep your application details'}).waitFor();
const saved=await page.evaluate(()=>JSON.parse(sessionStorage.getItem('setu-application')));
assert(saved.id && saved.accessKey);
await page.getByRole('button',{name:'Continue application & upload documents'}).click();
await page.locator('[name=contactPersonName]').fill('Updated');
await page.getByRole('button',{name:'Save application',exact:true}).click();
await page.getByText('Application saved. Upload supporting documents before submission.',{exact:true}).waitFor();
assert.equal(await page.locator('[name=contactPersonName]').inputValue(),'Updated');
await page.locator('[name=kind]').selectOption('registration');
await page.locator('[name=document]').setInputFiles({name:'test.pdf',mimeType:'application/pdf',buffer:Buffer.from('%PDF-1.4\nSynthetic test document\n%%EOF')});
await page.getByRole('button',{name:'Upload document',exact:true}).click();
await page.getByText('Document uploaded.',{exact:true}).waitFor();
await page.getByRole('button',{name:'Submit for review',exact:true}).click();
await page.getByText('Status: SUBMITTED',{exact:true}).waitFor();
await page.reload();await page.getByText('Status: SUBMITTED',{exact:true}).waitFor();
await page.screenshot({path:'/tmp/setu-application-desktop.png',fullPage:true});
const login=async account=>(await (await page.request.post('http://localhost:8080/api/local/session',{data:{account}})).json()).token;
const company=await login('company');
for(const action of ['review','approve']) {
 const r=await page.request.post(`http://localhost:8080/api/company/organizations/${saved.id}/${action}`,{headers:{Authorization:`Bearer ${company}`},data:{reason:''}});assert.equal(r.status(),200,await r.text());
}
for(let i=0;i<40;i++) {
 const r=await page.request.get(`http://localhost:8080/api/public/applications/${saved.id}`,{headers:{'X-Application-Key':saved.accessKey}});
 if((await r.json()).state==='ACTIVE') break;
 if(i===39) throw Error('Provisioning timed out');
 await new Promise(r=>setTimeout(r,1000));
}
const tenant=await login('alpha-admin');
await page.evaluate(token=>sessionStorage.setItem('setu-token',token),tenant);
await page.reload();await page.getByRole('button',{name:'Activate administrator access'}).click();
await page.waitForURL('http://localhost:8080/');
await page.getByRole('heading',{name:'Organization BROWSER-TEST',exact:true,level:1}).waitFor();
assert.equal(await page.evaluate(()=>sessionStorage.getItem('setu-application')),null);
await page.evaluate(()=>sessionStorage.clear());
await page.setViewportSize({width:390,height:844});await page.goto('http://localhost:8080');
await page.getByRole('heading',{name:'Sign in to Learning Setu'}).waitFor();
assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
await page.screenshot({path:'/tmp/setu-login-mobile.png',fullPage:true});
await page.locator('.apply-primary').click();
assert.equal(await page.locator('header .header-link').count(),0);
assert.equal(await page.getByRole('heading',{name:'Continue an application',exact:true}).count(),0);await page.getByRole('button',{name:'Start application',exact:true}).click();
assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
await page.screenshot({path:'/tmp/setu-application-mobile.png',fullPage:true});
assert.deepEqual(errors,[]);
console.log('PASS: public form, PDF upload, submit, resume, approval, verified activation, desktop/mobile layout, no browser errors.');
await browser.close();
