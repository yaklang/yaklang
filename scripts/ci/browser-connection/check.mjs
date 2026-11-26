import assert from 'node:assert/strict';
import { mkdir, mkdtemp, readFile, writeFile } from 'node:fs/promises';
import { createServer } from 'node:http';
import { join, resolve } from 'node:path';
import { runInNewContext } from 'node:vm';
import { extractFile } from '@electron/asar';
import { chromium } from 'playwright-core';
import { findLocalPort, startEngine, until } from './engine.mjs';

const fixtures = resolve(process.argv[2]);
const root = await mkdtemp(join(fixtures, 'run-'));
const logs = join(root, 'logs');
await mkdir(logs);
console.log('Fixture logs: ' + logs);
const files = {
  legacy: join(fixtures, 'legacy-yak.exe'),
  rejected: join(fixtures, 'alpha1123-yak.exe'),
  candidate: join(fixtures, 'candidate-yak.exe'),
  oldPlugin: join(fixtures, 'old-plugin'),
  newPlugin: join(fixtures, 'new-plugin'),
};

function memfitDecoder() {
  // These boundaries belong to the digest-pinned Memfit 1.1.1-1120 installer.
  // Exercise its shipped validator rather than recoding its schema in this test.
  const bundle = extractFile(join(fixtures, 'memfit.asar'), join('app', 'renderer', 'pages', 'main', 'assets', 'duplex-nQMSP18z.js')).toString('utf8');
  const start = bundle.indexOf(',he=262144,M=1048576');
  const end = bundle.indexOf('function Oe(e){', start);
  assert(start >= 0 && end > start, 'could not locate the released Memfit browser decoder');
  return runInNewContext('const ' + bundle.slice(start + 1, end) + '; De', { TextEncoder, TextDecoder });
}
const decode = memfitDecoder();

function readInMemfit(snapshot) {
  for (const record of snapshot.records) decode(record.type, record.raw);
}

function checkReleasedBaseline(snapshot) {
  for (const record of snapshot.records) {
    const emptyConnection = record.type === 'status' ? record.data.connections.findIndex(item => !item.clientVersion) : -1;
    const missing = record.type === 'status'
      ? (emptyConnection >= 0 ? '$.connections[' + emptyConnection + '].clientVersion' : undefined)
      : (!record.data.clientVersion ? '$.clientVersion' : undefined);
    if (missing) {
      // Older released plugins may omit their pre-bootstrap browser version.
      // Accept only this known failure while setting up the released baseline.
      assert.throws(() => decode(record.type, record.raw), {
        schema: 'snapshot.' + record.type, path: missing, code: 'browser_protocol_schema_mismatch',
      });
    } else {
      decode(record.type, record.raw);
    }
  }
}

async function extensionContext({ name, plugin, version, profile = name }) {
  const manifest = JSON.parse(await readFile(join(plugin, 'manifest.json'), 'utf8'));
  assert.equal(manifest.version, version);
  const context = await chromium.launchPersistentContext(join(root, profile), {
    executablePath: process.env.BROWSER_CONNECTION_CHROMIUM || chromium.executablePath(),
    headless: true,
    args: ['--no-first-run', '--disable-background-networking',
      '--disable-extensions-except=' + plugin, '--load-extension=' + plugin],
  });
  try {
    const worker = context.serviceWorkers()[0] ?? await context.waitForEvent('serviceworker', { timeout: 15_000 });
    const id = new URL(worker.url()).host;
    assert.equal(id, 'mcnaombmlombekhbonfndagbcfhmoail', 'fixture extension ID changed');
    const options = await context.newPage();
    await options.goto('chrome-extension://' + id + '/options.html');
    assert.equal(await options.evaluate(() => chrome.runtime.getManifest().version), version);
    const site = await context.newPage();
    const uri = base + '/' + name;
    await site.goto(uri);
    const send = (action, payload) => options.evaluate(async ({ action, payload }) => {
      const reply = await chrome.runtime.sendMessage({ action, payload });
      if (!reply?.ok) throw new Error(JSON.stringify(reply?.error ?? action));
      return reply.data;
    }, { action, payload });
    return { name, uri, send, close: () => context.close() };
  } catch (error) {
    await context.close();
    throw error;
  }
}

async function activeConnection(engine, plugin) {
  const installationId = (await plugin.send('state.get')).bridge.installationId;
  return until(plugin.name + ' signed connection', async () => {
    const snapshot = await engine.snapshot();
    const connection = snapshot.status.connections.find(record => record.installationId === installationId);
    const status = await plugin.send('bridge.status');
    if (!connection || status.state !== 'connected' || status.connectionId !== connection.connectionId) return;
    assert.equal(status.engineInstanceId, snapshot.status.engineInstanceId);
    return { snapshot, connection, status };
  });
}

async function pair(engine, plugin, transport, checkSnapshot = readInMemfit) {
  const { status } = await engine.snapshot();
  const { bridge } = await plugin.send('state.get');
  assert(!bridge.pairedEngine, plugin.name + ' must start with an unpaired test profile');
  let target;
  if (transport === 'native') {
    assert.match(status.url, /^npipe:/);
    target = await until('YTray Native Host discovering the candidate', async () => {
      const targets = await plugin.send('bridge.native.discover');
      return targets.find(item => item.engineIdentityId === status.engineIdentityId
        && item.engineInstanceId === status.engineInstanceId);
    });
    await plugin.send('bridge.config.save', {
      ...bridge, transport: 'native', nativeHost: 'com.yaklang.ytray_engine',
      launchTarget: target, autoConnect: false,
    });
  } else {
    assert.match(status.url, /^ws:\/\/127\.0\.0\.1:/);
    await plugin.send('bridge.config.save', { ...bridge, transport: 'websocket', endpoint: status.url, autoConnect: false });
  }
  checkSnapshot(await engine.snapshot('POST', '/pairing-window', { ttlSeconds: 90 }));
  await plugin.send(target ? 'bridge.native.pair' : 'bridge.pair', target && { engineInstanceId: target.engineInstanceId });
  const pending = await until(plugin.name + ' pending approval', async () => {
    const snapshot = await engine.snapshot();
    const request = snapshot.pending.find(item => item.installationId === bridge.installationId);
    if (!request) return;
    checkSnapshot(snapshot);
    return request;
  });
  checkSnapshot(await engine.snapshot('POST', '/pairings/' + pending.id + '/approve', { name: plugin.name }));
  const active = await activeConnection(engine, plugin);
  checkSnapshot(active.snapshot);
  return active;
}

async function reconnect(engine, plugin, expectedDeviceId, transport) {
  const { bridge } = await plugin.send('state.get');
  assert.equal(bridge.transport, transport);
  assert(bridge.pairedEngine, plugin.name + ' browser profile lost its pairing');
  await plugin.send('bridge.connect');
  const active = await activeConnection(engine, plugin);
  assert.equal(active.connection.deviceId, expectedDeviceId, plugin.name + ' pairing was replaced');
  assert.equal(active.snapshot.pending.length, 0);
  return active;
}

async function checkTabs(engine, plugin, deviceId, unexpected) {
  const tabs = await engine.browserTabs(deviceId);
  assert(tabs.some(tab => tab.url === plugin.uri), plugin.name + ' tab was not visible over the plugin bridge');
  if (unexpected) assert(!tabs.some(tab => tab.url === unexpected.uri), 'connected instance crossed into another Chrome profile');
}

async function closePhase(engine, ...plugins) {
  await Promise.allSettled(plugins.filter(Boolean).map(plugin => plugin.close()));
  await engine.close();
}

async function beforeOnline(engine, count, checkSnapshot = readInMemfit) {
  const snapshot = await engine.snapshot();
  checkSnapshot(snapshot);
  assert.equal(snapshot.devices.length, count);
  assert.equal(snapshot.status.connections.length, 0);
}

const fixtureServer = createServer((request, response) => {
  response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
  response.end('<html><title>Yak browser compatibility fixture</title><p>Fixture page</p></html>');
});
await new Promise((pass, fail) => fixtureServer.once('error', fail).listen(0, '127.0.0.1', pass));
const base = 'http://127.0.0.1:' + fixtureServer.address().port;


try {
  const wsHome = join(root, 'ws-engine');
  const wsPort = await findLocalPort();
  const legacy = await startEngine({
    executable: files.legacy, home: wsHome, logs, name: 'beta7', bridgePort: wsPort, legacy: true,
  });
  const oldProfile = { name: 'old-extension', plugin: files.oldPlugin, version: '0.2.11' };
  const newProfile = { name: 'new-extension', plugin: files.newPlugin, version: '0.2.22' };
  let oldDevice;
  let newDevice;
  let oldBrowser;
  let newBrowser;
  try {
    await beforeOnline(legacy, 0);
    oldBrowser = await extensionContext(oldProfile);
    oldDevice = (await pair(legacy, oldBrowser, 'websocket', checkReleasedBaseline)).connection.deviceId;
    newBrowser = await extensionContext(newProfile);
    newDevice = (await pair(legacy, newBrowser, 'websocket', checkReleasedBaseline)).connection.deviceId;
    assert.notEqual(oldDevice, newDevice);
    await checkTabs(legacy, oldBrowser, oldDevice, newBrowser);
    await checkTabs(legacy, newBrowser, newDevice, oldBrowser);
    assert.equal((await legacy.snapshot()).status.connections.length, 2);
    console.log('beta7 WebSocket: the two released extensions paired and read only their own tabs.');
  } finally {
    await closePhase(legacy, oldBrowser, newBrowser);
  }

  // The released alpha accepted the persisted pairing and the real plugin's
  // authentication, then broke the released desktop parser once online.
  const alpha = await startEngine({
    executable: files.rejected, home: wsHome, logs, name: 'alpha1123', bridgePort: wsPort,
  });
  oldBrowser = undefined;
  try {
    await beforeOnline(alpha, 2, checkReleasedBaseline);
    oldBrowser = await extensionContext(oldProfile);
    const active = await reconnect(alpha, oldBrowser, oldDevice, 'websocket');
    assert.throws(() => readInMemfit(active.snapshot), {
      schema: 'snapshot.status', path: '$.connections[0].reference', code: 'browser_protocol_schema_mismatch',
    });
    await checkTabs(alpha, oldBrowser, oldDevice);
    console.log('alpha1123 negative control: plugin online and callable, Memfit rejects status.connections[0].reference.');
  } finally {
    await closePhase(alpha, oldBrowser);
  }

  const ws = await startEngine({
    executable: files.candidate, home: wsHome, logs, name: 'candidate-websocket', bridgePort: wsPort,
  });
  oldBrowser = undefined;
  newBrowser = undefined;
  try {
    await beforeOnline(ws, 2);
    oldBrowser = await extensionContext(oldProfile);
    readInMemfit((await reconnect(ws, oldBrowser, oldDevice, 'websocket')).snapshot);
    newBrowser = await extensionContext(newProfile);
    const active = await reconnect(ws, newBrowser, newDevice, 'websocket');
    readInMemfit(active.snapshot);
    assert.equal(active.snapshot.status.connections.length, 2);
    await checkTabs(ws, oldBrowser, oldDevice, newBrowser);
    await checkTabs(ws, newBrowser, newDevice, oldBrowser);
    const freshOldBrowser = await extensionContext({ ...oldProfile, name: 'old-extension-fresh' });
    try {
      const fresh = await pair(ws, freshOldBrowser, 'websocket');
      assert.equal(fresh.snapshot.status.connections.length, 3);
      await checkTabs(ws, freshOldBrowser, fresh.connection.deviceId, oldBrowser);
    } finally {
      await freshOldBrowser.close();
    }
    console.log('Candidate WebSocket: both pairings retained; old plugin also paired afresh while both profiles were online.');
  } finally {
    await closePhase(ws, oldBrowser, newBrowser);
  }

  const nativeHome = join(root, 'native-engine');
  const nativeProfiles = ['native-a', 'native-b'].map(name => ({ ...newProfile, name }));
  const native = await startEngine({ executable: files.candidate, home: nativeHome, logs, name: 'candidate-native' });
  const nativeDevices = [];
  const nativeBrowsers = [];
  try {
    await beforeOnline(native, 0);
    for (const profile of nativeProfiles) {
      const browser = await extensionContext(profile);
      nativeBrowsers.push(browser);
      const active = await pair(native, browser, 'native');
      assert.equal(active.status.transport, 'named-pipe');
      nativeDevices.push(active.connection.deviceId);
    }
    assert.notEqual(nativeDevices[0], nativeDevices[1]);
    await checkTabs(native, nativeBrowsers[0], nativeDevices[0], nativeBrowsers[1]);
    await checkTabs(native, nativeBrowsers[1], nativeDevices[1], nativeBrowsers[0]);
    console.log('Candidate default Native: YTray host paired two real Chrome profiles, both decoded by old Memfit.');
  } finally {
    await closePhase(native, ...nativeBrowsers);
  }

  const restarted = await startEngine({ executable: files.candidate, home: nativeHome, logs, name: 'candidate-native-restart' });
  const resumed = [];
  try {
    await beforeOnline(restarted, 2);
    for (const [index, profile] of nativeProfiles.entries()) {
      const browser = await extensionContext(profile);
      resumed.push(browser);
      const active = await reconnect(restarted, browser, nativeDevices[index], 'native');
      assert.equal(active.status.transport, 'named-pipe');
      readInMemfit(active.snapshot);
    }
    const snapshot = await restarted.snapshot();
    assert.equal(snapshot.status.connections.length, 2);
    // Repeat the real online records at the observed 17-instance failure
    // threshold, without running 17 Chrome processes on a small runner.
    const manyInstances = {
      ...snapshot.status,
      connections: Array.from({ length: 17 }, (_, index) => ({
        ...snapshot.status.connections[index % 2],
        connectionId: 'fixture-connection-' + index, deviceId: 'fixture-device-' + index,
      })),
    };
    assert.equal(decode('status', JSON.stringify(manyInstances)).connections.length, 17);
    for (const connection of snapshot.status.connections) {
      assert(!Object.hasOwn(connection, 'capabilityCatalog'), 'desktop response duplicates the full signed catalog');
    }
    await checkTabs(restarted, resumed[0], nativeDevices[0], resumed[1]);
    await checkTabs(restarted, resumed[1], nativeDevices[1], resumed[0]);
    console.log('Candidate Native restart: both original device IDs online again and routed to their original profiles.');
  } finally {
    await closePhase(restarted, ...resumed);
  }
} finally {
  fixtureServer.close();
}
