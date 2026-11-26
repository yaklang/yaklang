import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { createWriteStream } from 'node:fs';
import { mkdir, readFile, unlink } from 'node:fs/promises';
import { createServer } from 'node:net';
import { join, resolve } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import grpc from '@grpc/grpc-js';
import { loadSync } from '@grpc/proto-loader';

const { ypb: { Yak } } = grpc.loadPackageDefinition(loadSync(
  resolve(import.meta.dirname, '../../../common/yakgrpc/yakgrpc.proto'), { keepCase: true },
));

export async function findLocalPort() {
  const server = createServer();
  await new Promise((pass, fail) => server.once('error', fail).listen(0, '127.0.0.1', pass));
  const port = server.address().port;
  await new Promise((pass, fail) => server.close(error => error ? fail(error) : pass()));
  return port;
}

export async function until(description, check, timeout = 25_000) {
  const deadline = Date.now() + timeout;
  let cause;
  do {
    try {
      const result = await check();
      if (result) return result;
    } catch (error) {
      cause = error;
    }
    await delay(150);
  } while (Date.now() < deadline);
  throw new Error('Timed out waiting for ' + description, { cause });
}

export async function startEngine({ executable, home, logs, name, bridgePort, legacy = false }) {
  await mkdir(home, { recursive: true });
  const port = await findLocalPort();
  const token = randomBytes(24).toString('hex');
  const args = ['grpc', '--host', '127.0.0.1', '--port', String(port), '--local-password', token, '--frontend', 'memfit'];
  if (bridgePort) {
    args.push('--browser-extension-bridge-port', String(bridgePort));
    if (!legacy) args.push('--browser-extension-bridge-transport', 'websocket');
  }
  // With no transport flag this exercises the candidate's Windows Native default.
  const child = spawn(executable, args, {
    env: { ...process.env, YAKIT_HOME: home, YAKIT_EDITION: 'memfit' },
    stdio: ['ignore', 'pipe', 'pipe'],
    windowsHide: true,
  });
  child.stdout.pipe(createWriteStream(join(logs, name + '-stdout.log')));
  child.stderr.pipe(createWriteStream(join(logs, name + '-stderr.log')));
  const ended = new Promise(done => child.once('exit', done).once('error', done));
  const rpc = new Yak('127.0.0.1:' + port, grpc.credentials.createInsecure(), {
    'grpc.max_receive_message_length': 16 * 1024 * 1024,
  });
  const auth = new grpc.Metadata();
  auth.set('authorization', 'Bearer ' + token);
  let lastStatus;

  async function snapshot(method = 'GET', path = '/snapshot', body) {
    const resources = await new Promise((pass, fail) => {
      rpc.RequestYakURL({
        Method: method,
        Url: { Schema: 'browser-extension', Location: 'local', Path: path },
        ...(body === undefined ? {} : { Body: Buffer.from(JSON.stringify(body)) }),
      }, auth, { deadline: Date.now() + 3000 }, (error, result) => {
        if (error) fail(error);
        else pass(result.Resources ?? []);
      });
    });
    const records = resources.map(resource => {
      const raw = resource.Extra.find(entry => entry.Key === 'data')?.Value;
      assert(raw, 'missing browser resource data');
      return { type: resource.ResourceType, raw, data: JSON.parse(raw) };
    });
    const status = records.find(record => record.type === 'status')?.data;
    assert(status, 'missing browser bridge status');
    lastStatus = status;
    return {
      records, status,
      devices: records.filter(record => record.type === 'paired-device').map(record => record.data),
      pending: records.filter(record => record.type === 'pairing-request').map(record => record.data),
    };
  }

  function browserTabs(deviceId) {
    return new Promise((pass, fail) => {
      let result;
      const request = rpc.ExecuteBrowserExtensionTask({
        DeviceId: deviceId, Schema: 'capability.call',
        Payload: Buffer.from(JSON.stringify({ method: 'browser.tabs', params: {} })),
        TimeoutMilliseconds: 8000,
      }, auth, { deadline: Date.now() + 10_000 });
      request.on('data', event => {
        if (event.Type === 'error' || event.Type === 'cancelled') fail(new Error(event.Message));
        if (event.Type !== 'result') return;
        try {
          result = JSON.parse(event.Data.toString('utf8'));
        } catch (error) {
          fail(error);
        }
      });
      request.on('error', fail);
      request.on('end', () => {
        if (Array.isArray(result)) pass(result);
        else fail(new Error('Browser did not return a tab list'));
      });
    });
  }

  async function close() {
    rpc.close();
    if (child.exitCode === null) child.kill();
    await ended;
    // A Windows process killed by a test can leave a Native discovery file.
    const pipe = lastStatus?.url?.match(/^npipe:(yak-browser-bridge-v1-engine-[0-9a-f]{32})$/)?.[1];
    if (pipe) {
      const descriptor = join(process.env.LOCALAPPDATA, 'YTray', 'BrowserEngines', pipe + '.json');
      try {
        const entry = JSON.parse(await readFile(descriptor, 'utf8'));
        if (entry.pid === child.pid && entry.engineInstanceId === lastStatus.engineInstanceId) await unlink(descriptor);
      } catch (error) {
        if (error.code !== 'ENOENT') throw error;
      }
    }
  }

  try {
    await until(name + ' engine startup', async () => (await snapshot()).status.running, 45_000);
    return { snapshot, browserTabs, close };
  } catch (error) {
    await close();
    throw error;
  }
}
