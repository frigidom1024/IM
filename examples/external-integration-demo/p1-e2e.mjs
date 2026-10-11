import { execFileSync } from 'node:child_process';
import { API_URL, fetchJSON, waitForHTTP, gochat, SocketClient, authenticate, assert } from './e2e.mjs';

const container = process.env.GOCHAT_CONTAINER || 'gochat-integration-gochat';
const json = (method, body) => ({ method, body: JSON.stringify(body) });
async function expectStatus(path, token, expected, options = {}) {
  const {response, body} = await fetchJSON(API_URL + path, { ...options, headers: {Authorization:`Bearer ${token}`, ...options.headers} });
  assert(response.status === expected, `${path}: ${response.status}, expected ${expected}: ${JSON.stringify(body)}`);
}
async function upload(token, bytes, type, mime, name, groupID=0) {
  const form = new FormData(); form.set('type',type); form.set('file',new Blob([bytes],{type:mime}),name);
  if (groupID) form.set('group_id',String(groupID));
  const {response,body}=await fetchJSON(API_URL+'/api/upload',{method:'POST',headers:{Authorization:`Bearer ${token}`},body:form});
  assert(response.ok && body.code===0,`upload ${type}: ${JSON.stringify(body)}`); return body.data;
}
async function download(path,token,expected=200) {
  const res=await fetch(API_URL+path,{headers:{Authorization:`Bearer ${token}`}});
  assert(res.status===expected,`download status ${res.status}: ${path}`);
  return Buffer.from(await res.arrayBuffer());
}

async function main() {
  await waitForHTTP(API_URL+'/readyz');
  const alice=await authenticate('demo-alice'), bob=await authenticate('demo-bob');
  let aliceSocket,bobSocket;
  const label=`p1-${Date.now()}`;
  try {
    // Persist an upload offset, restart the actual service, then resume.
    let session=(await gochat('/api/upload/session',alice.token,json('POST',{name:'resumed.txt',type:'file',mime:'text/plain',size:11}))).data;
    const path='/api/upload/session/'+session.id;
    const chunk=async(offset,data)=>(await gochat(path,alice.token,{method:'PATCH',headers:{'Content-Type':'application/octet-stream','Upload-Offset':String(offset)},body:data})).data;
    await chunk(0,'hello ');
    await expectStatus(path,bob.token,404);
    execFileSync('docker',['restart',container],{stdio:'pipe',timeout:30000});
    await waitForHTTP(API_URL+'/readyz');
    session=(await gochat(path,alice.token)).data;assert(session.offset===6,'upload offset lost after restart');
    await expectStatus(path,alice.token,409,{method:'PATCH',headers:{'Content-Type':'application/octet-stream','Upload-Offset':'0'},body:'again'});
    await chunk(6,'world');
    const file=(await gochat(path+'/complete',alice.token,{method:'POST'})).data;
    assert((await download(file.url,alice.token)).toString()==='hello world','resumed file bytes corrupted');
    await download(file.url,bob.token,403);
    console.log('✓ 上传偏移跨进程重启恢复，分块冲突被拒绝，文件内容与权限正确');

    aliceSocket=new SocketClient(alice.token,'P1 Alice');bobSocket=new SocketClient(bob.token,'P1 Bob');
    await Promise.all([aliceSocket.opened,bobSocket.opened]);
    async function send(socket,from,to,groupID,content,type='text',expected='msg_reply') {
      const seq=`${label}-${Math.random()}`;
      socket.socket.send(JSON.stringify({cmd:'msg',seq,data:{from_id:from,to_id:to,group_id:groupID,type,content}}));
      return socket.waitFor(message=>message.seq===seq && message.cmd===expected);
    }
    await send(bobSocket,bob.user.id,alice.user.id,0,JSON.stringify(file),'file','error');
    await download(file.url,bob.token,403);
    await send(aliceSocket,alice.user.id,bob.user.id,0,JSON.stringify(file),'file');
    assert((await download(file.url,bob.token)).toString()==='hello world','authorized file sharing failed');

    const group=(await gochat('/api/group/create',alice.token,json('POST',{name:label,max_members:5}))).data;
    const gp='/api/group/'+group.id;
    await gochat(gp+'/invite',alice.token,json('POST',{user_id:bob.user.id}));
    await gochat(gp+'/announcement',alice.token,json('PUT',{announcement:'P1 announcement'}));
    await gochat(gp+'/members/'+bob.user.id,bob.token,json('PATCH',{nickname:'Reader'}));
    await gochat(gp+'/members/'+bob.user.id,alice.token,json('PATCH',{muted_until:new Date(Date.now()+60000).toISOString()}));
    await send(bobSocket,bob.user.id,0,group.id,'muted','text','error');
    await gochat(gp+'/members/'+bob.user.id,alice.token,json('PATCH',{unmute:true}));
    const ack=await send(aliceSocket,alice.user.id,0,group.id,JSON.stringify(file),'file');
    await gochat('/api/message/read',bob.token,json('POST',{group_id:group.id,message_id:ack.data.message_id_string}));
    const receipts=(await gochat(`${gp}/messages/${ack.data.message_id_string}/receipts`,alice.token)).data;
    assert(receipts.some(member=>member.user_id===bob.user.id),'group read receipt missing');
    const files=(await gochat(gp+'/files',bob.token)).data;
    assert(files.some(item=>item.url===file.url),'group-shared file missing from group list');
    const search=(await gochat('/api/group/search?q='+label,bob.token)).data;
    assert(search.some(item=>item.id===group.id && item.announcement==='P1 announcement'),'group search missing announcement');
    await gochat('/api/conversation/private:'+bob.user.id,alice.token,json('PATCH',{pinned:true,muted:true,draft:'draft after restart'}));
    const conversations=(await gochat('/api/conversation',alice.token)).data;
    const privateConversation=conversations.find(item=>item.id==='private:'+bob.user.id);
    assert(privateConversation?.pinned && privateConversation.muted && privateConversation.draft==='draft after restart','conversation preferences missing');
    const bobConversations=(await gochat('/api/conversation',bob.token)).data;
    assert(!bobConversations.find(item=>item.id==='private:'+alice.user.id)?.draft,'draft leaked across users');
    await gochat(gp+'/transfer',alice.token,json('POST',{user_id:bob.user.id}));
    await expectStatus(gp,alice.token,403,{...json('DELETE',{}),headers:{'Content-Type':'application/json'}});
    await gochat(gp,bob.token,{method:'DELETE'});
    console.log('✓ 会话偏好隔离，群禁言/回执/文件列表/群主转让/解散闭环通过');

    const png=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=','base64');
    const image=await upload(alice.token,png,'image','image/png','pixel.png');
    assert(image.width===1 && image.height===1 && image.thumbnail,'image metadata/thumbnail missing');
    assert((await download(image.thumbnail,alice.token)).subarray(0,2).equals(Buffer.from([255,216])),'thumbnail is not JPEG');
    const voice=execFileSync('docker',['exec',container,'ffmpeg','-v','error','-f','lavfi','-i','sine=frequency=440:sample_rate=16000','-t','1','-f','mp3','pipe:1'],{timeout:30000,maxBuffer:2<<20});
    const audio=await upload(alice.token,voice,'voice','audio/mpeg','tone.mp3');
    assert(audio.duration>0.9 && audio.duration<1.5,'audio duration missing');
    const video=execFileSync('docker',['exec',container,'ffmpeg','-v','error','-f','lavfi','-i','color=c=black:s=64x64:r=5','-t','1','-c:v','libx264','-threads','1','-movflags','frag_keyframe+empty_moov','-f','mp4','pipe:1'],{timeout:30000,maxBuffer:2<<20});
    const movie=await upload(alice.token,video,'video','video/mp4','video.mp4');
    assert(movie.mime==='video/mp4' && movie.duration>0 && movie.width===64 && movie.thumbnail,'video conversion metadata missing');
    assert((await download(movie.url,alice.token)).length>0,'transcoded video empty');
    console.log('✓ PNG 缩略图、音频时长、真实 ffmpeg 视频转码及鉴权下载通过');

    // Use the isolated Docker database to advance expiry, then execute the
    // production maintenance binary against actual S3 HTTP object storage.
    const assetID=file.url.split('/')[3];
    assert(/^[a-f0-9]{32}$/.test(assetID),'invalid asset fixture ID');
    execFileSync('docker',['exec','-e','MYSQL_PWD=root','gochat-integration-mysql','mysql','-uroot','gochat','-e',`UPDATE media_asset SET expires_at=DATE_SUB(NOW(),INTERVAL 1 HOUR) WHERE id='${assetID}'; UPDATE upload_session SET expires_at=DATE_SUB(NOW(),INTERVAL 1 HOUR) WHERE id='${session.id}';`],{stdio:'pipe',timeout:30000});
    await download(file.url,alice.token,410);
    const storageURL=process.env.GOCHAT_S3_URL || 'http://localhost:19000';
    const fixtureIDs=Array.from({length:101},(_,index)=>index.toString(16).padStart(32,'0'));
    const orphanID='ffffffffffffffffffffffffffffffff';
    await Promise.all([...fixtureIDs,orphanID].map(async id=>{
      const response=await fetch(`${storageURL}/gochat/attachments/${id}`,{method:'PUT',headers:{'Content-Type':'text/plain','X-GoChat-Test-Modified':'2010-01-01T00:00:00Z'},body:'fixture'});
      assert(response.ok,'stale object fixture failed');
    }));
    const values=fixtureIDs.map(id=>`('${id}',${alice.user.id},'attachments/${id}','fixture','text/plain',7,NOW())`).join(',');
    execFileSync('docker',['exec','-e','MYSQL_PWD=root','gochat-integration-mysql','mysql','-uroot','gochat','-e',`INSERT IGNORE INTO media_asset(id,user_id,object_path,name,mime,size,created_at) VALUES ${values};`],{stdio:'pipe',timeout:30000});
    execFileSync('docker',['exec',container,'./im-maintenance'],{stdio:'pipe',timeout:30000});
    await download(file.url,alice.token,404);
    await expectStatus(path,alice.token,404);
    assert((await fetch(`${storageURL}/gochat/attachments/${orphanID}`)).status===404,'orphan after 101 permanent assets was skipped');
    assert((await fetch(`${storageURL}/gochat/attachments/${fixtureIDs[0]}`)).ok,'permanent asset deleted by maintenance');
    console.log('✓ 过期资产拒绝下载，维护命令清除数据库、临时文件和对象');

    const metrics=await (await fetch(API_URL+'/metrics')).text();
    assert(metrics.includes('gochat_http_requests_total') && metrics.includes('gochat_messages_total'),'Prometheus metrics missing');
    await expectStatus('/api/unknown',alice.token,404);
    const openapi=await (await fetch(API_URL+'/openapi.json')).json();assert(openapi.openapi && openapi.paths['/api/upload/session'],'OpenAPI route missing');
    console.log('P1 E2E passed.');
  } finally {aliceSocket?.close();bobSocket?.close();}
}
main().catch(error=>{console.error('P1 E2E failed:',error);process.exitCode=1;});
