// 最小 RESP 客户端：查询匹配队列与锁状态
import net from 'node:net';

const sock = net.createConnection({ host: '100.66.1.4', port: 6379 });
let buf = '';
const pending = [];

function send(cmd) {
  return new Promise((resolve) => {
    pending.push(resolve);
    const parts = cmd.split(' ');
    let out = `*${parts.length}\r\n`;
    for (const p of parts) out += `$${Buffer.byteLength(p)}\r\n${p}\r\n`;
    sock.write(out);
  });
}

sock.on('data', (d) => {
  buf += d.toString();
  let idx;
  while ((idx = buf.indexOf('\r\n')) >= 0) {
    const line = buf.slice(0, idx);
    buf = buf.slice(idx + 2);
    const res = pending.shift();
    if (res) res(line);
  }
});

await new Promise((r) => (sock.on('connect', r)));
await send('AUTH wangguiyou123');

console.log('match lock:', await send('GET penguin:match:lock'));
console.log('lock TTL:', await send('TTL penguin:match:lock'));
console.log('queue size:', await send('ZCARD penguin:match:queue'));
console.log('queue entries:', await send('ZRANGE penguin:match:queue 0 -1'));
sock.end();
