import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { mkdirSync, writeFileSync } from 'node:fs';
const { chromium } = createRequire(import.meta.url)('playwright');
const root = new URL('..', import.meta.url).pathname;
const output = root + 'output/visual-check';
mkdirSync(output, { recursive: true });
mkdirSync(root + 'frontend/public/artwork', { recursive: true });
const browser = await chromium.launch({ headless: true, channel: 'chrome' });
const errors = [];
try {
  const page = await browser.newPage({ viewport: { width: 1440, height: 1000 }, deviceScaleFactor: 1 });
  page.on('pageerror', error => { errors.push(error.message); console.error(error.message); });
  page.on('console', msg => { if (msg.type() === 'error') console.error(msg.text()); });
  page.on('requestfailed', request => console.error(request.url(), request.failure()));
  // Expose the app only in this browser test; production has no test entry point.
  await page.route('**/src/main.ts*', async route => {
    const response = await route.fetch();
    await route.fulfill({ response, body: (await response.text()).replace('new App();', 'window.__app = new App();') });
  });
  await page.goto(process.env.GAME_URL || 'http://127.0.0.1:5174');
  await page.waitForFunction(() => window.__app, null, { timeout: 10000 });
  const assets = await page.evaluate(async () => {
    const { penguinArtwork, iceArtwork } = await import('/src/render/artwork.ts');
    return { 'penguin-black': penguinArtwork(0).toDataURL(), 'penguin-orange': penguinArtwork(1).toDataURL(), 'ice-floe': iceArtwork().toDataURL() };
  });
  for (const [name, data] of Object.entries(assets)) {
    writeFileSync(`${root}frontend/public/artwork/${name}.png`, Buffer.from(data.split(',')[1], 'base64'));
  }
  await page.click('#btn-local');
  await page.evaluate(async () => {
    const G = await import('/src/core/game.ts');
    let state = G.createGame(['黑企鹅', '橙企鹅']);
    const positions = [9, 6, 16, 50, 26, 35, 45, 58];
    positions.forEach(i => state.tiles[i].fish = 1);
    for (const to of positions) state = G.applyMove(state, { kind: 'place', player: state.turn, from: -1, to });
    const app = window.__app;
    app.state = state;
    app.renderer.resize(state);
    app.resetSelection();
    app.refreshHUD();
  });
  await page.waitForTimeout(100);
  await page.screenshot({ path: `${output}/desktop.png` });
  const frame = await page.evaluate(() => {
    const app = window.__app, renderer = app.renderer;
    const canvas = document.querySelector('#board');
    const { width, height } = canvas.getBoundingClientRect();
    const points = app.state.tiles.map((_, i) => renderer.centerOf(i));
    const data = canvas.getContext('2d').getImageData(0, 0, canvas.width, canvas.height).data;
    const colors = new Set();
    for (let i = 0; i < data.length; i += 400) colors.add(`${data[i] >> 4},${data[i + 1] >> 4},${data[i + 2] >> 4}`);
    return { colors: colors.size, bounded: points.every(p => p.x > renderer.size && p.y > renderer.size && p.x < width - renderer.size && p.y < height - renderer.size) };
  });
  await page.screenshot({ path: `${output}/desktop.png` });
  assert.ok(frame.colors > 30 && frame.bounded, JSON.stringify({ frame, errors }));
  const selected = await page.evaluate(async () => {
    const G = await import('/src/core/game.ts');
    const app = window.__app;
    const move = G.allMoves(app.state, app.state.turn).find(m => Math.abs(m.to - m.from) > 4);
    app.onCellClick(move.from);
    return move;
  });
  await page.waitForTimeout(50);
  await page.screenshot({ path: `${output}/selected.png` });
  await page.evaluate(move => window.__app.onCellClick(move.to), selected);
  await page.waitForTimeout(110);
  await page.screenshot({ path: `${output}/cracking.png` });
  await page.waitForTimeout(230);
  await page.screenshot({ path: `${output}/shattering.png` });
  await page.waitForFunction(() => !window.__app.renderer.isAnimating);
  assert.equal(await page.evaluate(i => window.__app.state.tiles[i].gone, selected.from), true);
  await page.screenshot({ path: `${output}/moved.png` });

  for (const viewport of [{ width: 390, height: 844 }, { width: 360, height: 640 }, { width: 844, height: 390 }]) {
    await page.setViewportSize(viewport);
    await page.waitForTimeout(100);
    assert.ok(await page.locator('#status-card').isVisible());
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
    const size = await page.locator('#board').boundingBox();
    assert.ok(size.width > 200 && size.height > 100);
    await page.screenshot({ path: `${output}/mobile-${viewport.width}.png` });
  }
  await page.setViewportSize({ width: 390, height: 844 });
  await page.evaluate(async () => {
    const G = await import('/src/core/game.ts');
    const state = G.createGame(['黑企鹅', '橙企鹅']);
    state.tiles.forEach((tile, i) => { tile.gone = ![0, 35, 36, 37].includes(i); tile.owner = -1; tile.fish = 1; });
    state.tiles[0].owner = 0;
    state.tiles[35].owner = 1;
    state.players[0].penguins = [0]; state.players[1].penguins = [35];
    state.phase = 'moving'; state.turn = 1;
    const app = window.__app;
    app.state = state; app.renderer.resize(state); app.resetSelection(); app.refreshHUD();
    app.onCellClick(35); app.onCellClick(36);
  });
  await page.waitForFunction(() => !window.__app.renderer.isAnimating);
  assert.match(await page.locator('#status-card').innerText(), /黑企鹅 已无路可走.*橙企鹅 继续行动/);
  assert.equal(await page.evaluate(() => window.__app.state.turn), 1);
  await page.screenshot({ path: `${output}/blocked-player.png` });
  await page.evaluate(() => { window.__app.onCellClick(36); window.__app.onCellClick(37); });
  assert.equal(await page.locator('#overlay').isVisible(), false);
  await page.waitForFunction(() => !document.querySelector('#overlay').classList.contains('hidden'));
  assert.equal(await page.evaluate(() => window.__app.state.winner), 1);
  await page.evaluate(() => document.querySelector('#btn-undo').click());
  assert.equal(await page.locator('#overlay').isVisible(), false);
  assert.equal(await page.evaluate(() => window.__app.renderer.isAnimating), false);

  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.evaluate(() => { window.__app.onCellClick(36); window.__app.onCellClick(37); });
  assert.equal(await page.evaluate(() => window.__app.renderer.isAnimating), false);
  await page.emulateMedia({ reducedMotion: 'no-preference' });
  await page.evaluate(async () => {
    const G = await import('/src/core/game.ts');
    const app = window.__app;
    app.startLocal();
    const state = G.createGame(['Black', 'Orange']);
    state.tiles.forEach((tile, i) => { tile.gone = ![0, 35, 36, 37].includes(i); tile.owner = -1; tile.fish = 1; });
    state.tiles[0].owner = 0; state.tiles[35].owner = 1;
    state.players[0].penguins = [0]; state.players[1].penguins = [35];
    state.phase = 'moving'; state.turn = 1;
    app.state = state; app.renderer.resize(state);
    const first = { kind: 'move', player: 1, from: 35, to: 36 };
    const second = { kind: 'move', player: 1, from: 36, to: 37 };
    const next = G.applyMove(state, first), end = G.applyMove(next, second);
    app.handleServerMsg({ type: 'state', state: next, last: first });
    app.handleServerMsg({ type: 'over', state: end, last: second, winner: end.winner });
  });
  assert.equal(await page.evaluate(() => window.__app.state.phase), 'moving');
  assert.equal(await page.evaluate(() => window.__app.queuedStates.length), 1);
  await page.waitForFunction(() => !document.querySelector('#overlay').classList.contains('hidden'));
  assert.equal(await page.evaluate(() => window.__app.state.phase), 'finished');
  assert.equal(await page.evaluate(() => window.__app.queuedStates.length), 0);
  assert.equal(await page.evaluate(() => window.__app.renderer.penguins.every(img => img instanceof HTMLImageElement && img.naturalWidth > 0)), true);
  assert.deepEqual(errors, []);
  console.log(JSON.stringify({ result: 'passed', canvas: frame, screenshots: output, assets: Object.keys(assets), viewports: 4 }));
} finally {
  await browser.close();
}
