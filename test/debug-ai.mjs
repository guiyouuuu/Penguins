// 最小复现：人机对战放置阶段
const ws = new WebSocket('ws://localhost:5173/ws');
const log = (...a) => console.log(new Date().toISOString().slice(17, 23), ...a);

ws.onopen = () => {
  log('connected');
  ws.send(JSON.stringify({ type: 'play_ai' }));
};
ws.onmessage = async (ev) => {
  const m = JSON.parse(ev.data);
  if (m.type === 'start') {
    log('start, ply=', m.state.ply, 'turn=', m.state.turn);
    // 连续放 4 只（每次等 AI）
    placeNext(m.state);
  } else if (m.type === 'state') {
    log('state ply=', m.state.state?.ply ?? m.state.ply, 'phase=', m.state.phase, 'turn=', m.state.turn);
    if (m.state.phase === 'placing') placeNext(m.state);
    else {
      log('placing done, exiting');
      ws.close();
      process.exit(0);
    }
  } else {
    log('msg:', m.type, m.msg ?? '');
  }
};
ws.onerror = (e) => log('ws error', e.message ?? '');
ws.onclose = () => log('closed');

function placeNext(state) {
  // 找 1 鱼空格
  const tile = state.tiles.findIndex((t) => !t.gone && t.owner === -1 && t.fish === 1);
  log('place ->', tile);
  ws.send(JSON.stringify({ type: 'place', tile }));
}
setTimeout(() => {
  log('TIMEOUT - no completion in 15s');
  process.exit(1);
}, 15000);
