// Dependency-free experiment driver. Never run another benchmark, build or
// regression concurrently with `bench`; profile runs are separate observations.
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import crypto from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { spawn, execFileSync } from 'node:child_process';

const here = path.dirname(fileURLToPath(import.meta.url));
const cwd = path.resolve(here, '../..');
const [mode, binaryDir] = process.argv.slice(2);
if (!binaryDir || !path.isAbsolute(binaryDir)) throw new Error('mode and absolute frozen-binary directory required');
const reserved = mode.startsWith('reserved-');
const action = mode.replace(/^reserved-/, '');
const prefix = reserved ? 'reserved-' : '';
const candidate = reserved ? 'reserved' : 'batch';
const variants = ['baseline', 'journal', candidate];
const binary = variant => path.join(binaryDir, `${variant}.test`);
const sha = file => crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');

async function run(label, file, args, extraEnv = {}) {
  const output = path.join(here, `${label}.txt`);
  if (fs.existsSync(output)) throw new Error(`refusing to overwrite ${output}`);
  const fd = fs.openSync(output, 'wx');
  const started = new Date().toISOString();
  console.log(`START ${label} ${started}`);
  const status = await new Promise((resolve, reject) => {
    const child = spawn(file, args, { cwd, env: { ...process.env, ...extraEnv }, stdio: ['ignore', fd, fd] });
    child.on('error', reject);
    child.on('close', (code, signal) => resolve({ code, signal }));
  }).finally(() => fs.closeSync(fd));
  const event = { label, file, args, extraEnv, cwd, started, finished: new Date().toISOString(), ...status };
  fs.appendFileSync(path.join(here, 'commands.jsonl'), `${JSON.stringify(event)}\n`);
  console.log(`END ${label} exit=${status.code} signal=${status.signal}`);
  if (status.code !== 0) throw new Error(`${label} failed; original output retained`);
}

if (action === 'identity') {
  const sources = ['parser/base/config_store.go', 'parser/base/node.go', 'parser/base/config_batch.go',
    'parser/base/config_replay_test.go', 'parser/base/config_batch_test.go',
    'parser/stream_parser/expression_config.go', 'parser/stream_parser/tls_certificate_tree.go',
    'parser/stream_parser/field_batch_test.go', 'protocol_corpus_current_benchmark_test.go',
    'protocol_corpus_export_digest_test.go', 'parser/stream_parser/operator_worker.go'];
  const identity = { measuredAt: new Date().toISOString(), cwd, binaryDir,
    branch: execFileSync('git', ['branch', '--show-current'], { cwd, encoding: 'utf8' }).trim(),
    head: execFileSync('git', ['rev-parse', 'HEAD'], { cwd, encoding: 'utf8' }).trim(),
    dirty: true, go: execFileSync('go', ['version'], { encoding: 'utf8' }).trim(),
    os: execFileSync('sw_vers', [], { encoding: 'utf8' }).trim(),
    cpu: os.cpus()[0].model, cpus: os.cpus().length, memoryBytes: os.totalmem(),
    binaries: Object.fromEntries(variants.map(v => [v, { path: binary(v), sha256: sha(binary(v)) }])),
    manifestSHA256: sha(path.join(cwd, 'testdata/protocol-corpus/manifest.json')),
    currentSources: Object.fromEntries(sources.map(f => [f, sha(path.join(cwd, f))])),
    controlSources: Object.fromEntries(['config_store.baseline.go', 'node.baseline.go', 'expression_config.baseline.go',
      'tls_certificate_tree.baseline.go', 'config_store.journal.go', 'node.journal.go'].map(f => [f, sha(path.join(binaryDir, f))])) };
  fs.writeFileSync(path.join(here, `${prefix}identity.json`), JSON.stringify(identity, null, 2) + '\n', { flag: 'wx' });
} else if (action === 'bench') {
  for (let round = 0; round < 3; round++) {
    for (let i = 0; i < 3; i++) {
      const variant = variants[(round + i) % 3];
      await run(`${prefix}long-${round + 1}-${variant}`, binary(variant), ['-test.run', '^$', '-test.bench',
        '^BenchmarkCurrentCorpusApplication(Fields|JSON)$', '-test.benchtime=2s', '-test.count=1', '-test.cpu=1,10', '-test.benchmem']);
    }
  }
} else if (action === 'digest') {
  for (const variant of variants) await run(`${prefix}${variant}-export-digest`, binary(variant),
    ['-test.run', '^TestProtocolCorpusExportDigest$', '-test.count=1', '-test.v', '-test.timeout=180s'], { BINPARSER_EVALUATE: '1' });
} else if (action === 'corpus') {
  const all = execFileSync(binary(candidate), ['-test.list', '^TestProtocolCorpus'], { cwd, encoding: 'utf8' })
    .split('\n').filter(s => /^TestProtocolCorpus\w+$/.test(s));
  // Dedicated deferred-protocol checks are outside this iteration. Inventory
  // and envelope checks still retain every capture; no fixture is removed.
  const deferred = /^TestProtocolCorpus(?:DoQ|DoH|DoT|HTTP3|T38|AnyDesk|DingTalk|IMAPS|SMTPS|WeChat|MicroMsg|IQIYI|TencentGames|NetEaseGames|HoYoverse|Mihoyo|ClassifierFullCaptureCoverage)/i;
  const selected = all.filter(s => !deferred.test(s) && s !== 'TestProtocolCorpusExportDigest');
  fs.writeFileSync(path.join(here, `${prefix}corpus-selection.json`), JSON.stringify({ selected, deferred: all.filter(s => deferred.test(s)) }, null, 2) + '\n', { flag: 'wx' });
  await run(`${prefix}corpus-regression`, binary(candidate), ['-test.run', `^(${selected.join('|')})$`, '-test.count=1', '-test.v', '-test.timeout=600s']);
} else if (action === 'profile') {
  await run(`${prefix}allocation-profile-run`, binary(candidate), ['-test.run', '^$', '-test.bench', '^BenchmarkCurrentCorpusApplicationJSON$',
    '-test.benchtime=3s', '-test.count=1', '-test.cpu=1', '-test.memprofile', path.join(binaryDir, `${candidate}.heap`)]);
  await run(`${prefix}allocation-top`, 'go', ['tool', 'pprof', '-sample_index=alloc_space', '-top', '-nodecount=30', binary(candidate), path.join(binaryDir, `${candidate}.heap`)]);
} else if (action === 'summary') {
  const results = {};
  const median = values => [...values].sort((a, b) => a - b)[Math.floor(values.length / 2)];
  for (const variant of variants) {
    const groups = {};
    for (let round = 1; round <= 3; round++) {
      const lines = fs.readFileSync(path.join(here, `${prefix}long-${round}-${variant}.txt`), 'utf8').split('\n');
      for (const line of lines) {
        const match = line.match(/^BenchmarkCurrentCorpusApplication(Fields|JSON)(?:-(\d+))?\s+\d+\s+([\d.]+) ns\/op.*?([\d.]+) B\/op\s+([\d.]+) allocs\/op$/);
        if (!match) continue;
        const [, kind, worker = '1', ns, bytes, allocs] = match;
        (groups[`${kind}/${worker}`] ??= []).push({ ns: +ns, bytes: +bytes, allocs: +allocs, round });
      }
    }
    if (Object.keys(groups).length !== 4) throw new Error(`missing groups: ${variant}`);
    results[variant] = Object.fromEntries(Object.entries(groups).map(([key, rows]) => {
      if (rows.length !== 3) throw new Error(`missing rounds: ${variant}/${key}`);
      const ns = median(rows.map(r => r.ns));
      return [key, { rows, medianNs: ns, minNs: Math.min(...rows.map(r => r.ns)), maxNs: Math.max(...rows.map(r => r.ns)),
        medianBytes: median(rows.map(r => r.bytes)), medianAllocs: median(rows.map(r => r.allocs)),
        messagesPerSecond: 11e9 / ns, applicationInputMbps: 1421 * 8 * 1e3 / ns }];
    }));
  }
  const gains = Object.fromEntries(Object.keys(results.baseline).map(key => [key, {
    journalThroughputPercent: 100 * (results.baseline[key].medianNs / results.journal[key].medianNs - 1),
    batchAdditionalThroughputPercent: 100 * (results.journal[key].medianNs / results[candidate][key].medianNs - 1),
    totalThroughputPercent: 100 * (results.baseline[key].medianNs / results[candidate][key].medianNs - 1),
    allocatedBytesReductionPercent: 100 * (1 - results[candidate][key].medianBytes / results.baseline[key].medianBytes),
    allocationCountReductionPercent: 100 * (1 - results[candidate][key].medianAllocs / results.baseline[key].medianAllocs) }]));
  const digests = Object.fromEntries(variants.map(v => [v, [...fs.readFileSync(path.join(here, `${prefix}${v}-export-digest.txt`), 'utf8')
    .matchAll(/CURRENT_EXPORT_DIGEST workload=\w+ worker=\d+ records=\d+ sha256=[a-f0-9]+/g)].map(m => m[0]).sort()]));
  if (digests.baseline.length !== 8 || variants.some(v => JSON.stringify(digests[v]) !== JSON.stringify(digests.baseline))) {
    throw new Error('missing or unequal full-output digest partitions');
  }
  const summary = { batchMessages: 11, batchInputBytes: 1421, candidate, results, gains, digests };
  fs.writeFileSync(path.join(here, `${prefix}summary.json`), JSON.stringify(summary, null, 2) + '\n');
  console.log(JSON.stringify({ results, gains }, null, 2));
} else throw new Error('expected identity, bench, digest, corpus or summary');
