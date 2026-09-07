// Offline comparison of every retained PR #5023 generated record against an
// isolated run. Capture timestamps are intentionally excluded from packet
// equality; bytes, captured/wire lengths and link types are not normalized.
import { createHash } from 'node:crypto';
import { readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const digest = (data, algorithm = 'sha256') => createHash(algorithm).update(data).digest('hex');

export function readClassicPcap(data) {
  if (data.length < 24) throw new Error('incomplete classic-pcap header');
  const magic = data.subarray(0, 4).toString('hex');
  const little = magic === 'd4c3b2a1' || magic === '4d3cb2a1';
  if (!little && magic !== 'a1b2c3d4' && magic !== 'a1b23c4d') throw new Error('not classic pcap');
  const u16 = offset => little ? data.readUInt16LE(offset) : data.readUInt16BE(offset);
  const u32 = offset => little ? data.readUInt32LE(offset) : data.readUInt32BE(offset);
  if (u16(4) !== 2 || u16(6) !== 4) throw new Error('unsupported pcap version');
  const records = [];
  for (let offset = 24; offset < data.length;) {
    if (offset + 16 > data.length) throw new Error('incomplete pcap record header');
    const captured = u32(offset + 8), wireLength = u32(offset + 12);
    if (captured > wireLength || captured > u32(16)) throw new Error('invalid captured length');
    offset += 16;
    if (captured > data.length - offset) throw new Error('incomplete pcap record data');
    records.push({ wireLength, bytes: data.subarray(offset, offset + captured) });
    offset += captured;
  }
  return { linkType: u32(20), records };
}

export function byteDifferences(original, regenerated) {
  const ranges = [];
  for (let offset = 0; offset < Math.max(original.length, regenerated.length);) {
    if (original[offset] === regenerated[offset]) { offset++; continue; }
    const start = offset;
    while (offset < Math.max(original.length, regenerated.length) && original[offset] !== regenerated[offset]) offset++;
    ranges.push({ offset: start, length: offset - start,
      original_hex: original.subarray(start, offset).toString('hex'),
      regenerated_hex: regenerated.subarray(start, offset).toString('hex') });
  }
  return ranges;
}

export async function audit(corpus, reproduction) {
  const manifest = JSON.parse(await readFile(path.join(corpus, 'manifest.json'), 'utf8'));
  const recipePath = 'tools/generate-pr5023/generate.py';
  const recipe = await readFile(path.join(corpus, recipePath));
  const regeneratedRecipe = await readFile(path.join(reproduction, recipePath));
  if (!recipe.equals(regeneratedRecipe)) throw new Error('reproduction used a different script');
  const inputPath = 'tools/generate-pr5023/mariadb-greeting.bin';
  if (!(await readFile(path.join(corpus, inputPath))).equals(await readFile(path.join(reproduction, inputPath)))) {
    throw new Error('reproduction used a different MariaDB input');
  }
  const index = JSON.parse(await readFile(path.join(reproduction, 'tools/generate-pr5023/generated-index.json'), 'utf8'));
  const retained = manifest.captures.filter(c => c.repository_id === 'generated-pr5023');
  if (retained.length !== 168) throw new Error(`expected 168 retained captures, found ${retained.length}`);
  const captureResults = [], retainedIDs = new Set();
  let retainedRecords = 0, identicalRecords = 0, changedRecords = 0;
  for (const capture of retained) {
    const originalID = capture.id.replace(/^pr5023-/, '');
    if (!/^gen-[a-z0-9-]+$/.test(originalID)) throw new Error('unexpected generated capture ID');
    retainedIDs.add(originalID);
    const source = path.resolve(corpus, capture.capture_file);
    if (!source.startsWith(path.resolve(corpus, 'captures') + path.sep)) throw new Error('capture path leaves corpus');
    const originalBytes = await readFile(source);
    if (digest(originalBytes) !== capture.sha256) throw new Error(`${capture.id}: original digest changed`);
    const original = readClassicPcap(originalBytes);
    if (original.records.length !== capture.packet_count) throw new Error(`${capture.id}: original record count changed`);
    retainedRecords += original.records.length;
    const entry = { id: capture.id, original_sha256: capture.sha256, original_records: original.records.length };
    let regeneratedBytes;
    try { regeneratedBytes = await readFile(path.join(reproduction, 'captures/generated-local', originalID + '.pcap')); }
    catch (error) {
      if (error.code !== 'ENOENT') throw error;
      entry.outcome = 'missing';
      entry.reason = index.failed.find(c => c.id === originalID)?.error ?? 'no generated output';
      captureResults.push(entry);
      continue;
    }
    const regenerated = readClassicPcap(regeneratedBytes);
    entry.regenerated_sha256 = digest(regeneratedBytes);
    entry.regenerated_records = regenerated.records.length;
    entry.original_link_type = original.linkType;
    entry.regenerated_link_type = regenerated.linkType;
    entry.changed_records = [];
    for (let i = 0; i < Math.max(original.records.length, regenerated.records.length); i++) {
      const before = original.records[i], after = regenerated.records[i];
      if (before && after && before.wireLength === after.wireLength && before.bytes.equals(after.bytes)) {
        identicalRecords++;
      } else {
        changedRecords++;
        entry.changed_records.push({ frame: i + 1,
          original_wire_length: before?.wireLength ?? null, regenerated_wire_length: after?.wireLength ?? null,
          original_capture_length: before?.bytes.length ?? null, regenerated_capture_length: after?.bytes.length ?? null,
          ranges: byteDifferences(before?.bytes ?? Buffer.alloc(0), after?.bytes ?? Buffer.alloc(0)) });
      }
    }
    entry.outcome = entry.changed_records.length === 0 && original.linkType === regenerated.linkType ? 'packet-identical' : 'packet-different';
    captureResults.push(entry);
  }
  const counts = Object.fromEntries(['packet-identical', 'packet-different', 'missing'].map(k => [k, captureResults.filter(c => c.outcome === k).length]));
  return { schema_version: 1, recipe: recipePath, recipe_sha1: digest(recipe, 'sha1'),
    method: 'Compare every record byte and length plus link type; ignore only pcap capture timestamps. No original is rewritten and no differing bytes are normalized.',
    generated_kept: index.kept.length, generated_dropped: index.failed.length,
    retained_captures: retained.length, retained_records: retainedRecords, capture_outcomes: counts,
    identical_records: identicalRecords, changed_records: changedRecords,
    extra_generated_ids: index.kept.map(c => c.id).filter(id => !retainedIDs.has(id)).sort(),
    captures: captureResults };
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const corpus = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
  const [reproduction, output] = process.argv.slice(2);
  if (!reproduction || !output) throw new Error('usage: node audit-pr5023-reproduction.mjs ISOLATED_CORPUS OUTPUT_JSON');
  if (path.resolve(reproduction) === corpus) throw new Error('reproduction must be isolated from the original corpus');
  const result = await audit(corpus, path.resolve(reproduction));
  await writeFile(output, JSON.stringify(result, null, 2) + '\n');
  console.log(JSON.stringify({ captures: result.retained_captures, records: result.retained_records, outcomes: result.capture_outcomes,
    identical_records: result.identical_records, changed_records: result.changed_records, extra_outputs: result.extra_generated_ids.length }));
  // A report was written, but a non-identical reproduction is not success.
  if (result.capture_outcomes['packet-different'] || result.capture_outcomes.missing) process.exitCode = 1;
}
