import assert from 'node:assert/strict';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

// Run against TestBrowserLobbyFixture through the Vite /api and /ws proxy.
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE });
const base = process.env.PENGUIN_FRONTEND_URL || 'http://127.0.0.1:5173';
async function login(page, name) {
  await page.locator('#auth-modal').waitFor({state:'visible'});
  await page.locator('#auth-username').fill(name);
  await page.locator('#auth-password').fill('fixture-password');
  await page.locator('#auth-submit').click();
}
async function menu(page) {
  await page.locator('#btn-exit').click();
  await page.getByRole('button', {name:'确定离开', exact:true}).click();
}
try {
  const hostContext = await browser.newContext({viewport:{width:1440,height:900}});
  const guestContext = await browser.newContext({viewport:{width:390,height:844}});
  const host = await hostContext.newPage();
  const guest = await guestContext.newPage();
  const errors = [];
  for (const page of [host,guest]) {
    page.setDefaultTimeout(10000);
    page.on('pageerror',e=>errors.push(e.message));
  }
  await host.goto(base);
  await host.locator('#btn-create').click();
  await login(host,'Host');
  await host.locator('#lobby-host').filter({hasText:'Host'}).waitFor();
  const code = await host.locator('#room-code').inputValue();
  assert.match(code,/^[ABCDEFGHJKMNPQRSTUVWXYZ23456789]{4}$/);
  assert.equal(await host.locator('#btn-start-game').isEnabled(),false);
  await guest.goto(`${base}/?room=${code}`);
  assert.equal(await guest.locator('#room-input').inputValue(),code);
  await guest.locator('#btn-join-confirm').click();
  await guest.locator('#auth-cancel').click();
  assert.equal(await guest.locator('#room-input').inputValue(),code);
  await guest.locator('#btn-join-confirm').click();
  await login(guest,'Guest');
  await host.locator('#lobby-guest').filter({hasText:'Guest'}).waitFor();
  assert.equal(await guest.locator('#game-screen').isVisible(),false);
  assert.equal(await host.locator('#btn-start-game').isEnabled(),false);
  await guest.locator('#btn-cancel-wait').click();
  await host.locator('#lobby-guest').filter({hasText:'空位'}).waitFor();
  assert.equal(await host.locator('#room-code').inputValue(),code);
  await guest.locator('#btn-join-confirm').click();
  await guest.locator('#guest-ready').check();
  await host.locator('#lobby-guest-status').filter({hasText:'已准备'}).waitFor();
  await guest.locator('#guest-ready').uncheck();
  await host.locator('#lobby-guest-status').filter({hasText:'未准备'}).waitFor();
  await guest.locator('#guest-ready').check();
  await host.locator('#lobby-guest-status').filter({hasText:'已准备'}).waitFor();
  for (const [page, sizes] of [[host,[[1440,900],[320,568]]],[guest,[[390,844]]]]) {
    for (const [width,height] of sizes) {
      await page.setViewportSize({width,height});
      assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);
      const cancel = await page.locator('#btn-cancel-wait').boundingBox();
      assert.ok(cancel.y>=0 && cancel.y+cancel.height<=height,JSON.stringify(cancel));
      await page.screenshot({path:join(tmpdir(),`penguins-friend-lobby-${width}.png`)});
    }
  }
  await host.locator('#btn-start-game').click();
  await Promise.all([host,guest].map(p=>p.locator('#game-screen').waitFor({state:'visible'})));
  assert.equal(await host.locator('#game-subtitle').textContent(),'联机对战');
  assert.equal(await guest.locator('#game-subtitle').textContent(),'联机对战');
  await menu(guest);
  await host.locator('#overlay-title').filter({hasText:'对手离开了'}).waitFor();
  await host.getByRole('button',{name:'返回菜单',exact:true}).click();
  await guest.locator('#btn-join').click();
  await guest.locator('#room-input').fill(code);
  await guest.locator('#btn-join-confirm').click();
  await guest.locator('#menu-tip').filter({hasText:'房间不存在或已关闭'}).waitFor();
  await host.locator('#btn-ai').click();
  await host.locator('#game-screen').waitFor({state:'visible'});
  assert.equal(await host.locator('#game-subtitle').textContent(),'人机对战');
  await menu(host);
  await host.locator('#btn-match').click();
  await guest.locator('#btn-match').click();
  await Promise.all([host,guest].map(p=>p.locator('#game-screen').waitFor({state:'visible'})));
  assert.deepEqual(errors,[]);
  console.log('Two-browser workflow passed: login/cancel/resume, invite, leave/rejoin, ready/unready, host start, game exit, closed room, AI and quick match; desktop/mobile layout passed.');
} finally {
  await browser.close();
  if (process.env.PENGUIN_FIXTURE_DONE_URL) await fetch(process.env.PENGUIN_FIXTURE_DONE_URL);
}
