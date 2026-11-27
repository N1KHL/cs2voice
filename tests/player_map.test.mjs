import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const rootDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const playerPath = path.join(rootDir, 'frontend', 'player.js');
const statePath = path.join(rootDir, 'frontend', 'state.js');

const hasModules = fs.existsSync(playerPath) && fs.existsSync(statePath);
const playerMod = hasModules ? await import(pathToFileURL(playerPath).href) : null;
const stateMod = hasModules ? await import(pathToFileURL(statePath).href) : null;

const skipReason = hasModules ? false : 'awaiting refactor-frontend merge (frontend/player.js, frontend/state.js)';

test('fileToDemo: empty segments and single-segment clamping', { skip: skipReason }, () => {
  const { fileToDemo } = playerMod;

  assert.equal(fileToDemo([], 5), 0);
  assert.equal(fileToDemo(null, 5), 0);

  const segs = [{ out: 0, len: 10, demo: 100 }];
  assert.equal(fileToDemo(segs, -3), 100);
  assert.equal(fileToDemo(segs, 0), 100);
  assert.equal(fileToDemo(segs, 4.5), 104.5);
  assert.equal(fileToDemo(segs, 10), 110);
  assert.equal(fileToDemo(segs, 25), 110);
});

test('fileToDemo: multi-segment compact gaps and output gaps', { skip: skipReason }, () => {
  const { fileToDemo } = playerMod;

  // Compact mode: contiguous in file time, separated by gaps in demo time.
  const compactSegs = [
    { out: 0, len: 5, demo: 50 },
    { out: 5, len: 4, demo: 80 },
    { out: 9, len: 6, demo: 120 },
  ];
  assert.equal(fileToDemo(compactSegs, 2), 52);
  assert.equal(fileToDemo(compactSegs, 5), 55);
  assert.ok(Math.abs(fileToDemo(compactSegs, 5.25) - 80.25) < 1e-9);
  assert.equal(fileToDemo(compactSegs, 7), 82);
  assert.equal(fileToDemo(compactSegs, 12), 123);
  assert.equal(fileToDemo(compactSegs, 15), 126);
  assert.equal(fileToDemo(compactSegs, 99), 126);

  // Gap in file time before a segment clamps to that segment's demo start.
  const gappedOut = [
    { out: 2, len: 3, demo: 50 },
    { out: 7, len: 4, demo: 90 },
  ];
  assert.equal(fileToDemo(gappedOut, 1), 50);
  assert.equal(fileToDemo(gappedOut, 6), 90);
});

test('demoToFile: non-overlapping, clamped, and multi-segment ranges', { skip: skipReason }, () => {
  const { demoToFile, fileToDemo } = playerMod;

  assert.deepEqual(demoToFile([], 0, 10), []);
  assert.deepEqual(demoToFile(null, 0, 10), []);

  const segs = [
    { out: 0, len: 5, demo: 50 },
    { out: 5, len: 4, demo: 80 },
    { out: 9, len: 6, demo: 120 },
  ];

  // Outside or inside compact gaps
  assert.deepEqual(demoToFile(segs, 10, 40), []);
  assert.deepEqual(demoToFile(segs, 55, 80), []);
  assert.deepEqual(demoToFile(segs, 126, 140), []);
  assert.deepEqual(demoToFile(segs, 82, 82), []);
  assert.deepEqual(demoToFile(segs, 83, 81), []);

  // Single segment partial overlap
  assert.deepEqual(demoToFile(segs, 48, 52.5), [{ s: 0, e: 2.5 }]);
  assert.deepEqual(demoToFile(segs, 124, 135), [{ s: 13, e: 15 }]);

  // Multi-segment range spanning across compact gaps
  assert.deepEqual(demoToFile(segs, 52, 122), [
    { s: 2, e: 5 },
    { s: 5, e: 9 },
    { s: 9, e: 11 },
  ]);

  // Round-trip consistency for interior file positions
  const d0 = fileToDemo(segs, 6.5);
  const pieces = demoToFile(segs, d0, d0 + 1);
  assert.equal(pieces.length, 1);
  assert.ok(Math.abs(pieces[0].s - 6.5) < 1e-9);
  assert.ok(Math.abs(pieces[0].e - 7.5) < 1e-9);
});

test('waveHeight: scales monotonically with speaker lanes and caps for all-talk', { skip: skipReason }, () => {
  const { waveHeight } = playerMod;

  assert.equal(waveHeight(0), 76);
  assert.equal(waveHeight(1), 76);
  assert.equal(waveHeight(2), 84);
  assert.equal(waveHeight(3), 96);
  assert.equal(waveHeight(4), 108);
  assert.equal(waveHeight(5), 120);
  assert.equal(waveHeight(6), 132);
  assert.ok(waveHeight(6) > waveHeight(5));
  assert.equal(waveHeight(8), 156);
  assert.equal(waveHeight(9), 160);
  assert.equal(waveHeight(20), 160);
});

test('state.js: teamPlayers, talkIn, groupPlayers, setMatch, and reasonText', { skip: skipReason }, () => {
  const {
    setMatch,
    teamPlayers,
    talkIn,
    groupPlayers,
    BOTH,
    REASONS,
    reasonText,
  } = stateMod;

  const match = {
    teams: ['Team Alpha', 'Team Bravo', ''],
    players: [
      { id: 10, name: 'alice', team: 'Team Alpha', talk: 12 },
      { id: 11, name: 'bob', team: 'Team Alpha', talk: 0 },
      { id: 20, name: 'carol', team: 'Team Bravo', talk: 8 },
      { id: 30, name: 'spec', team: '', talk: 0 },
    ],
    periods: [
      { id: 1, kind: 'round', number: 1, talk: { 10: 4.5, 20: 3.0 } },
      { id: 2, kind: 'break', label: 'Halftime', allTalk: true },
    ],
  };

  setMatch(match);

  assert.equal(BOTH, '\u0000both');
  assert.deepEqual(teamPlayers('Team Alpha'), [10, 11]);
  assert.deepEqual(teamPlayers('Team Bravo', match), [20]);
  assert.deepEqual(teamPlayers('', match), [30]);
  assert.deepEqual(teamPlayers('Nonexistent', match), []);

  assert.equal(talkIn(match.periods[0], [10, 11, 20, 999]), 7.5);
  assert.equal(talkIn(match.periods[0], [11]), 0);
  assert.equal(talkIn(match.periods[1], [10, 20]), 0);

  const groups = groupPlayers();
  assert.equal(groups.length, 3);
  assert.equal(groups[0].team, 'Team Alpha');
  assert.deepEqual(groups[0].players.map(p => p.id), [10, 11]);
  assert.equal(groups[1].team, 'Team Bravo');
  assert.deepEqual(groups[1].players.map(p => p.id), [20]);
  assert.equal(groups[2].team, '');
  assert.deepEqual(groups[2].players.map(p => p.id), [30]);

  assert.equal(REASONS.length, 5);
  assert.equal(reasonText(new Set(), '   '), '');
  assert.equal(
    reasonText(new Set(['reason.griefing', 'reason.abuse']), '  trolling in spawn  '),
    'Verbal abuse / insults; Intentionally ruining the game; trolling in spawn'
  );
});
