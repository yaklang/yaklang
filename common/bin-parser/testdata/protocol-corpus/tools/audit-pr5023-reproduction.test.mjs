import assert from 'node:assert/strict';
import test from 'node:test';
import { readClassicPcap, byteDifferences } from './audit-pr5023-reproduction.mjs';

function pcap(little, payload = Buffer.from([0, 255, 128])) {
  const b = Buffer.alloc(40 + payload.length);
  const u16 = (value, offset) => little ? b.writeUInt16LE(value, offset) : b.writeUInt16BE(value, offset);
  const u32 = (value, offset) => little ? b.writeUInt32LE(value, offset) : b.writeUInt32BE(value, offset);
  u32(0xa1b2c3d4, 0); u16(2, 4); u16(4, 6); u32(65535, 16); u32(249, 20);
  u32(123, 24); u32(456, 28); u32(payload.length, 32); u32(payload.length + 2, 36);
  payload.copy(b, 40);
  return b;
}

test('both byte orders preserve complete record bytes and wire lengths', () => {
  for (const little of [true, false]) {
    const b = pcap(little);
    assert.deepEqual(readClassicPcap(b), { linkType: 249, records: [{ wireLength: 5, bytes: Buffer.from([0, 255, 128]) }] });
    const changedTime = Buffer.from(b);
    changedTime[24] ^= 127;
    assert.deepEqual(readClassicPcap(b), readClassicPcap(changedTime));
    for (let cut = 0; cut < b.length; cut++) {
      if (cut === 24) continue; // a complete, empty capture
      assert.throws(() => readClassicPcap(b.subarray(0, cut)), `prefix ${cut}`);
    }
  }
});

test('invalid lengths, signatures and versions cannot be accepted', () => {
  for (const change of [b => b[0] = 0, b => b[4] = 3, b => b.writeUInt32LE(2, 16), b => b.writeUInt32LE(2, 36)]) {
    const b = pcap(true); change(b); assert.throws(() => readClassicPcap(b));
  }
});

test('difference ranges include changed bytes, insertion tails and removal tails', () => {
  assert.deepEqual(byteDifferences(Buffer.from([0, 1, 2, 3, 4]), Buffer.from([0, 8, 9, 3, 4, 5])), [
    { offset: 1, length: 2, original_hex: '0102', regenerated_hex: '0809' },
    { offset: 5, length: 1, original_hex: '', regenerated_hex: '05' }
  ]);
  assert.deepEqual(byteDifferences(Buffer.from([255]), Buffer.alloc(0)), [{ offset: 0, length: 1, original_hex: 'ff', regenerated_hex: '' }]);
  assert.deepEqual(byteDifferences(Buffer.from([128]), Buffer.from([128])), []);
});
