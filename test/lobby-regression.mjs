import assert from 'node:assert/strict';
import { test } from 'node:test';
import { readFile } from 'node:fs/promises';
import { build } from '../frontend/node_modules/esbuild/lib/main.js';

async function load(path) {
  const result = await build({ entryPoints: [new URL(path, import.meta.url).pathname], bundle: true, format: 'esm', write: false });
  return import(`data:text/javascript;base64,${Buffer.from(result.outputFiles[0].text).toString('base64')}`);
}
const { App } = await load('../frontend/src/ui/app.ts');
const { NetClient } = await load('../frontend/src/net/client.ts');
const { createGame } = await load('../frontend/src/core/game.ts');
const html = await readFile(new URL('../frontend/index.html', import.meta.url), 'utf8');

function setup(url = 'https://penguins.iepose.cn/') {
  const elements = new Map([...html.matchAll(/id="([^"]+)"/g)].map(([, id]) => {
    const classes = new Set(['hidden']);
    const listeners = {};
    return [id, { value: '', textContent: '', disabled: false, listeners,
      classList: { add: c => classes.add(c), remove: c => classes.delete(c), contains: c => classes.has(c),
        toggle(c, force = !classes.has(c)) { if (force) classes.add(c); else classes.delete(c); } },
      addEventListener: (name, fn) => { listeners[name] = fn; }, focus() {}, select() {},
      click() { if (!this.disabled) return listeners.click?.(); },
    }];
  }));
  globalThis.document = { getElementById: id => elements.get(id) };
  globalThis.location = new URL(url);
  const sent = [];
  const app = Object.assign(Object.create(App.prototype), {
    mode: 'menu', lobbyAction: null, lobbyRoom: '', netRequest: 0, queuedStates: [],
    lobbyNames: ['', ''], guestReady: false, lobbyPending: false, loginPending: false,
    auth: { currentUser: { username: 'Host' } },
    renderer: { isAnimating: false }, net: { connected: true, close() {}, send: msg => sent.push(msg) },
  });
  app.bindMenu();
  app.bindNet();
  return { app, sent, el: id => elements.get(id) };
}

test('room code survives the following waiting message and has invitation commands', async () => {
  const { app, el } = setup();
  await app.startNet('create');
  app.handleServerMsg({ type: 'room', room: 'ABCD', you: 0 });
  app.handleServerMsg({ type: 'waiting' });
  assert.equal(el('room-code')?.value, 'ABCD');
  assert.equal(el('room-lobby').classList.contains('hidden'), false);
  assert.equal(el('room-share').classList.contains('hidden'), false);
  assert.equal(el('btn-create').disabled, true);
  assert.ok(el('btn-copy-invite').listeners.click);
});

test('invitation URL pre-fills join without sending a command', () => {
  const { el, sent } = setup('https://penguins.iepose.cn/?room=abcd');
  assert.equal(el('room-input').value, 'ABCD');
  assert.equal(el('join-row').classList.contains('hidden'), false);
  assert.equal(sent.length, 0);
});

test('duplicate clicks and cancellation cannot send a late create command', async () => {
  const { app, el, sent } = setup();
  let resolve;
  let connects = 0;
  app.net.connected = false;
  app.net.connect = () => { connects++; return new Promise(r => { resolve = r; }); };
  const first = app.startNet('create');
  const second = app.startNet('create');
  assert.equal(connects, 1);
  el('btn-cancel-wait').click();
  resolve();
  await Promise.all([first, second]);
  assert.equal(sent.length, 0);
  assert.equal(el('btn-create').disabled, false);
  app.net.connected = true;
  await app.startNet('create');
  assert.deepEqual(sent, [{ type: 'create_room' }]);
});

test('copy invite excludes unrelated query parameters and failure leaves selectable link', async () => {
  const { app, el } = setup('https://penguins.iepose.cn/?token=private');
  await app.startNet('create');
  app.handleServerMsg({ type: 'room', room: 'ABCD', you: 0 });
  let copied;
  Object.defineProperty(globalThis, 'navigator', { configurable: true, value: { clipboard: { writeText: async text => { copied = text; } } } });
  await el('btn-copy-invite').click();
  assert.equal(copied, 'https://penguins.iepose.cn/?room=ABCD');
  navigator.clipboard.writeText = async () => { throw new Error('denied'); };
  await el('btn-copy-invite').click();
  assert.equal(el('invite-link').value, copied);
  assert.equal(el('invite-fallback').classList.contains('hidden'), false);
});

test('server rejection and waiting disconnect allow retry', async () => {
  for (const event of ['error', 'close']) {
    const { app, el } = setup();
    await app.startNet('join', 'ABCD');
    if (event === 'error') app.handleServerMsg({ type: 'error', msg: '房间不存在' });
    else app.net.onClose();
    assert.equal(el('btn-create').disabled, false);
    assert.equal(el('room-lobby').classList.contains('hidden'), true);
    assert.ok(el('menu-tip').textContent);
  }
});

test('joining and AI start clear the lobby while preserving the game connection', async () => {
  for (const action of ['join', 'ai', 'match']) {
    const { app, el } = setup();
    let closes = 0;
    app.net.close = () => closes++;
    let entered;
    app.enterGame = subtitle => { entered = subtitle; };
    await app.startNet(action, 'ABCD');
    app.handleServerMsg({ type: 'room', room: 'ABCD', you: 1 });
    if (action === 'join') assert.equal(el('lobby-status').textContent, '正在同步房间…');
    else assert.equal(el('room-share').classList.contains('hidden'), true);
    const state = createGame(['Host', 'Guest']);
    app.handleServerMsg({ type: 'start', state });
    assert.equal(app.mode, action === 'ai' ? 'ai' : 'online');
    assert.equal(app.state, state);
    assert.equal(app.mySeat, 1);
    assert.ok(entered);
    assert.equal(el('room-lobby').classList.contains('hidden'), true);
    assert.equal(closes, 0);
  }
});

test('guest must log in and the original join resumes once, with its invitation intact', async () => {
  const { app, sent, el } = setup('https://penguins.iepose.cn/?room=ABCD');
  let login;
  app.auth.currentUser = null;
  app.auth.requestLogin = () => new Promise(resolve => { login = resolve; });
  const pending = app.startNet('join', el('room-input').value);
  await app.startNet('create');
  assert.equal(sent.length, 0);
  app.auth.currentUser = { username: 'Guest' };
  login(true);
  await pending;
  assert.deepEqual(sent, [{ type: 'join_room', room: 'ABCD' }]);
});

test('room lobby exposes host start only after guest ready and guest ready toggle', async () => {
  const { app, sent, el } = setup();
  await app.startNet('create');
  app.handleServerMsg({type:'room', room:'ABCD', you:0});
  app.handleServerMsg({type:'lobby', room:'ABCD', names:['Host','Guest'], ready:false});
  assert.equal(el('lobby-host').textContent, 'Host');
  assert.equal(el('lobby-guest').textContent, 'Guest');
  assert.equal(el('btn-start-game').disabled, true);
  app.handleServerMsg({type:'lobby', room:'ABCD', names:['Host','Guest'], ready:true});
  el('btn-start-game').click();
  assert.equal(sent.at(-1).type, 'start_game');
  app.handleServerMsg({type:'error', code:30011, msg:'请等待好友加入并准备'});
  assert.equal(el('room-lobby').classList.contains('hidden'), false);
  app.mySeat = 1;
  app.handleServerMsg({type:'lobby', room:'ABCD', names:['Host','Guest'], ready:false});
  el('guest-ready').checked = true;
  el('guest-ready').listeners.change();
  assert.deepEqual(sent.at(-1), {type:'ready', ready:true});
});

class FakeSocket {
  static OPEN = 1;
  static instances = [];
  readyState = 0;
  constructor() { FakeSocket.instances.push(this); }
  open() { this.readyState = 1; this.onopen?.(); }
  close() { this.readyState = 3; }
}
function network() {
  globalThis.WebSocket = FakeSocket;
  globalThis.localStorage = { getItem: () => null };
  globalThis.location = new URL('https://penguins.iepose.cn/');
  return new NetClient();
}

test('cancelled connection rejects and stale events cannot affect the next socket', async () => {
  const net = network();
  let closed = 0;
  let messages = 0;
  net.onClose = () => closed++;
  net.onMessage = () => messages++;
  const first = net.connect();
  const rejected = assert.rejects(first);
  const old = FakeSocket.instances.at(-1);
  net.close();
  const next = net.connect();
  FakeSocket.instances.at(-1).open();
  await next;
  old.onclose?.();
  old.onmessage?.({ data: '{"type":"waiting"}' });
  assert.equal(net.connected, true);
  assert.equal(messages, 0);
  FakeSocket.instances.at(-1).onclose();
  assert.equal(closed, 1);
  await rejected;
});
