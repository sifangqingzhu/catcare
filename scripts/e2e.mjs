import assert from 'node:assert/strict'
import { randomBytes } from 'node:crypto'
import { mkdir, writeFile } from 'node:fs/promises'
const base=process.env.TEST_BASE_URL||'http://127.0.0.1:8092'
const suffix=randomBytes(5).toString('hex'),password=randomBytes(16).toString('hex')
const actor=()=>({cookie:''}),a=actor(),b=actor(),c=actor(),anon=actor();let checks=0
async function request(who,path,method='GET',body,status=200,headers={}) {
 const res=await fetch(base+'/api'+path,{method,headers:{...headers,...(body?{'Content-Type':'application/json'}:{}),...(who.cookie?{Cookie:who.cookie}:{})},body:body?JSON.stringify(body):undefined})
 const data=await res.json();assert.equal(res.status,status,`${method} ${path}: ${data.error||'unexpected status'}`);checks++
 const cookie=res.headers.get('set-cookie');if(cookie)who.cookie=cookie.split(';')[0];return data
}
const now=new Date().toISOString()
async function analyzed(who,path) {
 const data=await request(who,path+'/data');const deadline=Date.now()+120000
 while(!data.analysis_job.has_result) {
  assert.ok(['queued','running'].includes(data.analysis_job.status),JSON.stringify(data.analysis_job))
  assert.ok(Date.now()<deadline,'background analysis exceeded test budget')
  await new Promise(resolve=>setTimeout(resolve,25))
  const status=await request(who,path+'/analysis/'+data.analysis_job.id)
  data.analysis_job=status.job;if(status.analysis)data.analysis=status.analysis
 }
 return data
}
const users=[]
await request(anon,'/me','GET',undefined,401)
for (const [who,role] of [[a,'owner'],[b,'editor'],[c,'viewer']]){const name=`qa_${role}_${suffix}`;const out=await request(who,'/auth/register','POST',{username:name,password,display_name:`QA ${role}`});who.name=name;users.push(out.user.id)}
await mkdir('.runtime',{recursive:true});await writeFile('.runtime/qa-users.json',JSON.stringify(users))
const t=await request(a,'/tables','POST',{name:`QA ${suffix}`,description:'Disposable integration test data',visibility:'private'},201),path=`/tables/${t.id}`
await request(b,path+'/data','GET',undefined,404)
await request(a,path,'PATCH',{name:t.name,description:t.description,visibility:'public'})
assert.equal((await request(b,path+'/data')).role,'public')
await request(b,path+'/cats','POST',{name:'Forbidden',sex:'unknown',color:'sage'},403)
await request(b,path+'/join','POST');let ms=await request(a,path+'/members');assert.equal(ms.find(m=>m.username===b.name).status,'pending')
await request(a,path+`/members/${users[1]}`,'PATCH',{status:'approved',role:'editor'})
const cat1=await request(a,path+'/cats','POST',{name:'QA one',breed:'',sex:'male',birthday:'',color:'sage',neutered:true,archived:false,notes:''})
const cat2=await request(b,path+'/cats','POST',{name:'QA two',breed:'',sex:'female',birthday:'',color:'peach',neutered:true,archived:false,notes:''})
const litter=await request(a,path+'/litters','POST',{name:'QA litter',material:'mixed',wait_minutes:10,notes:'same scale'})
const box=await request(a,path+'/boxes','POST',{name:'QA shared box',location:'QA',litter_id:litter.id,cat_ids:[cat1.id,cat2.id],archived:false})
const record={kind:'urine',cat_id:null,box_id:box.id,occurred_at:now,formed_at:new Date(new Date(now)-10*60000).toISOString(),integrity:'complete',collection_complete:true,count:1,grams:42,diameter_cm:4.5,looseness:2,moisture:null,shape:'',size:null,body_weight_kg:null,blood:false,straining:false,no_output:false,notes:'Unknown identity test'}
const r=await request(b,path+'/records','POST',record)
let data=await analyzed(a,path);assert.equal(data.analysis.matches.find(m=>m.record_id===r.id).status,'unknown')
await request(a,path+'/litters/'+litter.id,'PATCH',{name:'QA changed litter',material:'tofu',wait_minutes:20,notes:'new protocol'})
await request(b,path+'/records/'+r.id,'PATCH',{...record,cat_id:cat1.id})
data=await request(a,path+'/data');assert.equal(data.records.find(row=>row.id===r.id).litter_key,r.litter_key);assert.equal(data.records.find(row=>row.id===r.id).litter_name,'QA litter')
const nr=await request(a,path+'/records','POST',record);assert.notEqual(nr.litter_key,r.litter_key)
await request(a,path+'/records','POST',{...record,grams:-1},400)
await request(a,path+'/records','POST',{...record,occurred_at:new Date(Date.now()+86400000).toISOString()},400)
await request(a,path+'/records','POST',{...record,kind:'stool',diameter_cm:null,looseness:null},400)
await request(a,path+'/records','POST',{...record,kind:'stool',diameter_cm:null,looseness:null,moisture:3,shape:'formed',size:3})
const emergency=await request(a,path+'/records','POST',{...record,formed_at:null,count:0,grams:null,diameter_cm:null,looseness:null,straining:true,no_output:true})
data=await request(a,path+'/data');assert.ok(data.analysis.alerts.some(a=>a.record_id===emergency.id&&a.level==='urgent'&&a.cat_id===null))
await request(a,path+'/members','POST',{username:c.name,role:'viewer'})
await request(c,path+'/data');await request(c,path+'/weights','POST',{cat_id:cat1.id,kg:4.2,occurred_at:now,notes:''},403)
await request(c,path+'/records/'+r.id,'DELETE',undefined,403)
await request(c,path,'PATCH',{name:t.name,description:'',visibility:'private'},403)
const w=await request(b,path+'/weights','POST',{cat_id:cat1.id,kg:4.2,occurred_at:now,notes:''},201)
await request(b,path+'/weights/'+w.id,'DELETE')
const foreign=await request(b,'/tables','POST',{name:'QA foreign',description:'Disposable test',visibility:'private'},201)
const fc=await request(b,`/tables/${foreign.id}/cats`,'POST',{name:'Foreign cat',sex:'unknown',color:'blue'})
await request(a,path+'/records','POST',{...record,cat_id:fc.id},400)
await request(a,path+'/boxes','POST',{name:'Invalid cat ref',litter_id:litter.id,cat_ids:[fc.id]},400)
await request(a,path+`/cats/${fc.id}`,'PATCH',{name:'Illegal edit',sex:'unknown',color:'blue'},404)
await request(a,path+'/records','POST',record,403,{Origin:'https://other.example'})
await request(a,path+'/records','POST',{...record,mode:'shared',cat_id:cat1.id},400)
await request(a,path+'/records','POST',{...record,mode:'baseline'},400)
await request(a,path+'/records','POST',{...record,mode:'baseline',cat_id:cat1.id,count:2},400)
const currentCount=(await request(a,path+'/data')).records.length
await request(a,path+'/batches','POST',{items:[{...record,mode:'shared'},{...record,mode:'shared',cat_id:fc.id}]},400)
assert.equal((await request(a,path+'/data')).records.length,currentCount,'invalid batch must roll back all records')
for (const [cat,grams,diameter] of [[cat1,40,4],[cat2,80,7]]) {
 for(let day=0;day<7;day++) {
  const at=new Date(Date.now()-(12-day)*86400000).toISOString()
  await request(a,path+'/records','POST',{...record,mode:'baseline',identity_method:'isolated',cat_id:cat.id,grams,diameter_cm:diameter,occurred_at:at,formed_at:new Date(new Date(at)-10*60000).toISOString()})
 }
}
const batch=await request(a,path+'/batches','POST',{items:[[40,4],[40,4],[15,2],[15,2]].map(([grams,diameter_cm])=>({...record,mode:'shared',grams,diameter_cm}))},201)
const earlier=new Date(new Date(now)-2*3600000).toISOString()
await request(a,path+'/batches','POST',{items:[[40,4],[40,4],[15,2],[15,2]].map(([grams,diameter_cm])=>({...record,mode:'shared',grams,diameter_cm,occurred_at:earlier,formed_at:new Date(new Date(earlier)-10*60000).toISOString()}))},201)
assert.equal(batch.records.length,4);assert.ok(batch.records.every(r=>r.cat_id===null&&r.mode==='shared'&&r.batch_id===batch.batch_id))
const queued=await request(a,path+'/data');assert.equal(queued.analysis_job.has_result,false,'new snapshot must return before its fit');assert.equal(queued.analysis_job.status,'queued')
const jobID=queued.analysis_job.id
assert.equal((await request(a,path+'/data')).analysis_job.id,jobID,'same snapshot must deduplicate work')
await request(a,`/tables/${foreign.id}/analysis/${jobID}`,'GET',undefined,404)
const poolData=await analyzed(a,path)
const poolAlert=poolData.analysis.alerts.find(a=>a.metric==='shared_pool')
assert.ok(poolAlert,'shared novelty must alarm without attributed samples')
assert.ok(poolAlert.anchor_cat_id==null,'healthy observations cannot exclude either cat intermittent changes')
assert.equal(poolAlert.cat_id,null,'difference suggestion must never rewrite sample identities')
assert.equal(poolData.analysis.shared_anomalies,4)
assert.equal(poolAlert.localization,'candidate_set','retain source ambiguity without independent labels')
const pool=poolData.analysis.pools.find(p=>p.kind==='urine'&&p.box_id===box.id)
assert.equal(pool.model_version,'joint-mixture-v3')
assert.equal(pool.batch_analysis.batches,2)
assert.ok(pool.profiles.every(f=>f.quality.normal_validation==='assessed'&&f.quality.localization_validation==='unvalidated'))
const joint=poolData.analysis.joint_analyses[0]
assert.equal(joint.evaluated,4);assert.equal(joint.collections,2);assert.equal(joint.periods,2)
assert.ok(joint.candidate_ids.includes(cat1.id)&&joint.candidate_ids.includes(cat2.id))
assert.ok(joint.follow_ups.length>0);assert.equal(joint.validation,'unvalidated')
assert.equal((await request(a,path+'/analysis/'+jobID)).job.status,'done')
await request(a,path+'/records','POST',{...record,formed_at:new Date(Date.now()+60000).toISOString()},400)
await request(a,path+'/records','POST',{...record,last_cleaning_complete:true},400)
await request(a,path+'/records','POST',{...record,last_cleaned_at:new Date(new Date(now)-60000).toISOString(),last_cleaning_complete:true},400)
await request(a,path+'/records','POST',{...record,mode:'baseline',cat_id:cat1.id,identity_method:'manual'},400)
await request(a,path+'/records','POST',{...record,mode:'baseline',cat_id:cat1.id,integrity:'mixed'},400)
await request(a,path+'/records','POST',{...record,mode:'shared',identity_method:'witnessed'},400)
await request(a,path+'/records','POST',{...record,baseline_version:-1},400)
await request(a,path+'/batches','POST',{items:[{...record,mode:'shared'},{...record,mode:'shared',collection_complete:false}]},400)
const tagged=await request(a,path+'/records','POST',{...record,mode:'observed',cat_id:cat1.id,event_id:`event-${suffix}`,length_cm:4,width_cm:3,thickness_cm:2,litter_depth_cm:4})
assert.equal(tagged.length_cm,4);assert.equal(tagged.integrity,'complete')
await request(a,path+'/records','POST',{...record,cat_id:cat2.id,event_id:tagged.event_id},400)
await request(a,path+'/records','POST',{...record,cat_id:cat1.id,event_id:tagged.event_id,formed_at:new Date(new Date(now)-20*60000).toISOString()},400)
const mixed=await request(a,path+'/batches','POST',{items:[{...record,mode:'shared',grams:999,integrity:'mixed'}]},201)
assert.equal(mixed.records[0].cat_id,null)
await request(a,path+'/analysis/'+jobID,'GET',undefined,404)
const mixedPool=(await analyzed(a,path)).analysis.pools.find(p=>p.kind==='urine'&&p.box_id===box.id)
assert.equal(mixedPool.observations.find(o=>o.record_id===mixed.records[0].id).status,'unusable')
assert.ok(!poolData.analysis.matches.some(m=>batch.records.some(r=>r.id===m.record_id)),'normal shared samples do not require attribution')
await request(a,path,'PATCH',{name:t.name,description:t.description,visibility:'private'})
await request(c,path+'/data');await request(a,path+`/members/${users[1]}`,'PATCH',{role:'editor',status:'removed'})
await request(b,path+'/data','GET',undefined,404);await request(b,path+'/join','POST',undefined,404)
const accessible=await request(a,path+'/data');await request(b,path+'/analysis/'+accessible.analysis_job.id,'GET',undefined,404)
const retry=await request(c,path+'/analysis','POST');assert.ok(['queued','running','done'].includes(retry.analysis_job.status))
await request(a,path+'/records/'+r.id,'DELETE')
await request(a,'/auth/logout','POST');await request(a,'/me','GET',undefined,401)
console.log(`PASS: ${checks} HTTP checks; auth, permissions, isolated async jobs, invalidation, emergency, baseline quality, time metadata, event consistency, atomic collections and joint candidate sets.`)
