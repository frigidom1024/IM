import { writeFile } from 'node:fs/promises';
import { API_URL, gochat, SocketClient, authenticate, assert, waitForHTTP } from './e2e.mjs';

const json = (method, body) => ({ method, body: JSON.stringify(body) });
const label = `relations-${Date.now()}`;
async function builtin(suffix) {
  const username = `${label}-${suffix}`, password = 'RelationTest123456';
  const response = await fetch(`${API_URL}/api/auth/register`, { ...json('POST', { username, password, nickname: `关系验收 ${suffix}` }), headers: { 'Content-Type': 'application/json' } });
  assert(response.ok, 'fixture registration failed');
  const login = await fetch(`${API_URL}/api/auth/login`, { ...json('POST', { username, password }), headers: { 'Content-Type': 'application/json' } });
  const result = await login.json(); assert(result.code === 0, 'fixture login failed');
  return { ...result.data, username, password };
}
async function main() {
  await waitForHTTP(`${API_URL}/readyz`);
  const builtinMode = process.env.GOCHAT_AUTH_MODE === 'builtin';
  const a = builtinMode ? await builtin('A') : await authenticate('demo-rel-a');
  const b = builtinMode ? await builtin('B') : await authenticate('demo-rel-b');
  if (process.env.RELATION_FIXTURE_PATH) await writeFile(process.env.RELATION_FIXTURE_PATH, JSON.stringify({ a, b }));
  let as = new SocketClient(a.token, 'Relation A'), bs = new SocketClient(b.token, 'Relation B');
  await Promise.all([as.opened, bs.opened]);
  async function send(type, content, expected = 'msg_reply', groupID = 0, seq = crypto.randomUUID()) {
    as.socket.send(JSON.stringify({ cmd: 'msg', seq, data: { conversation_id: groupID ? `group:${groupID}` : `private:${b.user.id}`, type, content } }));
    return as.waitFor(msg => msg.seq === seq && msg.cmd === expected);
  }
  const conversationList = async who => (await gochat('/api/conversation', who.token)).data;
  try {
    await gochat('/api/friend/add', a.token, json('POST', { friend_id: b.user.id }));
    await bs.waitFor(msg => msg.cmd === 'friend_changed');
    assert((await conversationList(a)).length === 0 && (await conversationList(b)).length === 0, 'relationship created empty conversations');
    const notices = (await gochat('/api/friend/incoming', b.token)).data;
    assert(notices.some(item => item.user_id === a.user.id && item.unread), 'friend notice was not persisted');
    assert((await send('file', '{}', 'error')).data.code === 42011, 'intro allowed media');
    bs.close();
    const introSeq = crypto.randomUUID(); const first = await send('text', '首次问候', 'msg_reply', 0, introSeq);
    // Offline delivery is recovered through durable conversation + unread state.
    bs = new SocketClient(b.token, 'Relation B reconnected'); await bs.opened;
    const unread = (await gochat('/api/conversation/unread', b.token)).data;
    assert(unread.unread_list.some(item => item.id === `private:${a.user.id}` && item.unread_count === 1), 'pulled intro notification missing');
    assert((await conversationList(b)).some(item => item.id === `private:${a.user.id}` && item.unread_count === 1), 'nonfriend message hidden');
    assert((await send('text', 'second', 'error')).data.code === 42012, 'second intro accepted');
    await gochat(`/api/friend/${b.user.id}`, a.token, { method: 'DELETE' });
    await gochat('/api/friend/add', a.token, json('POST', { friend_id: b.user.id }));
    assert((await send('text', 'readded', 'error')).data.code === 42012, 'readding reset quota');
    const retry = await send('text', '首次问候', 'msg_reply', 0, introSeq);
    assert(retry.data.message_id_string === first.data.message_id_string, 'retry duplicated intro');
    await gochat('/api/friend/add', b.token, json('POST', { friend_id: a.user.id }));
    await send('text', '互加后的消息'); await send('file', '{}');
    await gochat(`/api/conversation/private:${a.user.id}/read`, b.token, { method: 'POST' });
    assert((await conversationList(b))[0].unread_count === 0, 'read state did not persist');
    console.log('✓ Docker 私信：好友通知与会话分离、离线首次文本、额度限制、重试、重加及互加权限');

    const group = (await gochat('/api/group/create', a.token, json('POST', { name: label, max_members: 2 }))).data;
    const gp = `/api/group/${group.id}`;
    let application = (await gochat(gp + '/join', b.token, { method: 'POST' })).data;
    assert(application.status === 'pending', 'join granted immediate membership');
    assert((await gochat('/api/group/list', b.token)).data.length === 0, 'pending applicant is a member');
    await gochat(`${gp}/applications/${application.id}/review`, a.token, json('POST', { decision: 'reject', reason: '重新提交' }));
    const originalID = application.id;
    application = (await gochat(gp + '/join', b.token, { method: 'POST' })).data;
    assert(application.id !== originalID, 'reapply overwrote rejection');
    await gochat(`${gp}/applications/${application.id}/review`, a.token, json('POST', { decision: 'approve' }));
    assert((await gochat('/api/group/applications/mine', b.token)).data.some(item => item.id === application.id && item.status === 'approved'), 'approval not visible');
    assert(!(await conversationList(b)).some(item => item.id === `group:${group.id}`), 'approval created empty conversation');
    await send('text', '群聊验收', 'msg_reply', group.id);
    assert((await conversationList(b)).some(item => item.id === `group:${group.id}`), 'group messages missing from mixed conversations');
    await gochat(gp + '/leave', b.token, { method: 'DELETE' });
    assert(!(await conversationList(b)).some(item => item.id === `group:${group.id}`), 'left group stayed in conversations');
    await gochat(gp + '/join', b.token, { method: 'POST' });
    await gochat(gp, a.token, { method: 'DELETE' });
    assert((await gochat('/api/group/applications/mine', b.token)).data.some(item => item.status === 'closed'), 'dissolution left pending application');
    console.log('✓ Docker 入群：审核、拒绝重申、申请状态、混合会话、退出与解散');
  } finally { as.close(); bs.close(); }
}
main().catch(error => { console.error('Relationship E2E failed:', error.message); process.exitCode = 1; });
