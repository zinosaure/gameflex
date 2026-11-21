// Live Wii smoke test. Credentials arrive on stdin and are never logged.
const fs = require('fs');
const WebSocket = require('/work/src/webapp/src/node_modules/ws');

const input = JSON.parse(fs.readFileSync(0, 'utf8'));
const config = Array.isArray(input)
  ? Object.fromEntries(input.map(value => {
      const separator = value.indexOf('=');
      return [value.slice(0, separator), value.slice(separator + 1)];
    }))
  : input;
if (Array.isArray(input)) {
  config.username = config.GAMEFLEX_ROOT_USERNAME;
  config.password = config.GAMEFLEX_ROOT_PASSWORD;
}
const base = 'http://gameflex-web:8080';

async function main() {
  const login = await fetch(base + '/api/v1/auth/login', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({username: config.username, password: config.password}),
  });
  if (!login.ok) throw new Error('login ' + login.status);
  const identity = await login.json();
  const token = identity.item.access_token;
  const headers = {Authorization: 'Bearer ' + token};
  const get = async path => {
    const response = await fetch(base + path, {headers});
    if (!response.ok) throw new Error('HTTP ' + response.status + ' for ' + path);
    return response.json();
  };
  for (const section of ['cores', 'bios']) {
    const response = await fetch(
      base + '/api/v1/retrogaming/listing/rescan?section=' + section,
      {method: 'POST', headers},
    );
    if (!response.ok) throw new Error('rescan ' + section + ' ' + response.status);
  }
  const roms = (await get('/api/v1/retrogaming/wii/roms')).data || [];
  const cores = (await get('/api/v1/retrogaming/wii/cores')).data || [];
  const game = roms.find(item => /mario kart wii/i.test(item.title)) || roms[0];
  const core = cores.find(item => item.name === 'dolphin') || cores[0];
  if (!game || !core) throw new Error('Wii ROM or Dolphin core unavailable after rescan');

  const facade = new WebSocket(
    base.replace('http', 'ws') + '/api/v1/facade/ws?token=' + encodeURIComponent(token),
  );
  let next = 0;
  const pending = new Map();
  facade.on('message', raw => {
    const frame = JSON.parse(raw.toString());
    if (frame.type === 'reply' && pending.has(frame.id)) {
      pending.get(frame.id)(frame);
      pending.delete(frame.id);
    }
  });
  await new Promise((resolve, reject) => {
    facade.once('open', resolve);
    facade.once('error', reject);
    setTimeout(() => reject(new Error('facade timeout')), 15000).unref();
  });
  const command = (type, data) => {
    const id = String(++next);
    return new Promise((resolve, reject) => {
      pending.set(id, resolve);
      facade.send(JSON.stringify({type, id, data}));
      setTimeout(() => reject(new Error(type + ' timeout')), 90000).unref();
    });
  };
  const launched = await command('launch', {
    platform: 'retrogaming', consoleId: 'wii', reference: game.reference,
    title: game.title, core: core.name, bios: '', resolution: 'sd', language: 'fr',
  });
  if (launched.status < 200 || launched.status >= 300) {
    throw new Error('launch refused with status ' + launched.status);
  }
  const data = launched.data?.data || launched.data;
  const sessionId = data.sessionId || data.session_id;
  if (!sessionId) throw new Error('launch returned no session');

  const stream = new WebSocket(
    base.replace('http', 'ws') + '/api/v1/retrogaming/stream/ws?token=' + encodeURIComponent(token),
  );
  await new Promise((resolve, reject) => {
    const timeout = setTimeout(() => reject(new Error('stream startup timeout')), 30000);
    stream.once('open', () => stream.send(JSON.stringify({
      type: 'start', sessionId, username: identity.item.user.username,
    })));
    stream.on('message', raw => {
      if (Buffer.isBuffer(raw) && raw[0] !== 0x7b) return;
      const message = JSON.parse(raw.toString());
      if (message.type === 'started') { clearTimeout(timeout); resolve(); }
      if (message.type === 'error') { clearTimeout(timeout); reject(new Error('stream refused')); }
    });
    stream.once('error', reject);
  });
  await new Promise(resolve => setTimeout(resolve, 20000));
  const alive = await fetch(base + '/api/v1/sessions/' + encodeURIComponent(sessionId), {headers});
  const stopped = await command('session.stop', {platform: 'retrogaming', sessionId});
  stream.close();
  facade.close();
  console.log(JSON.stringify({
    authentication: login.status,
    game: game.title,
    core: core.name,
    streamStarted: true,
    aliveAfterSeconds: alive.ok ? 20 : 0,
    stopStatus: stopped.status,
  }));
  if (!alive.ok || stopped.status < 200 || stopped.status >= 300) process.exitCode = 1;
}

main().catch(error => {
  console.log(JSON.stringify({verification: 'failed', reason: error.message}));
  process.exit(1);
});
