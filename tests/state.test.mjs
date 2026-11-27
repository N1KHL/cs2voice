import test from 'node:test';
import assert from 'node:assert/strict';
import { L, R, C, BOTH, setMatch, snapshotState, restoreState, activeTab } from '../frontend/state.js';

const match = () => ({
  duration: 600,
  teams: ['Alpha', 'Bravo'],
  players: [
    { id: '1', team: 'Alpha' },
    { id: '2', team: 'Alpha' },
    { id: '3', team: 'Bravo' },
  ],
  periods: [
    { id: 0, kind: 'warmup', number: 0, start: 0, allTalk: true, talk: {} },
    { id: 1, kind: 'round', number: 1, start: 60, allTalk: false, talk: { 1: 3 } },
    { id: 2, kind: 'break', number: 0, start: 300, allTalk: true, talk: {} },
  ],
});

test('a snapshot restores every panel on the same match', () => {
  setMatch(match());
  L.periods = new Set([1, 2]);
  L.players = new Set(['1']);
  L.mode = 'full';
  R.team = 'Bravo';
  R.periods = new Set([1]);
  R.audio = new Set(['3']);
  R.reported = new Set(['3']);
  R.format = 'wav';
  R.marks = [{ start: 30, end: 35 }, { start: 10, end: 12 }];
  R.reasons = new Set(['reason.abuse']);
  R.reasonOther = 'spam';
  R.reportEdited = true;
  R.reportText = 'edited report';
  C.team = 'Bravo';
  C.ranges = [{ start: 1, end: 2 }];
  const saved = JSON.parse(JSON.stringify(snapshotState()));

  setMatch(match()); // back to the defaults
  assert.equal(R.marks.length, 0);
  assert.equal(restoreState(saved), true);

  assert.deepEqual([...L.periods], [1, 2]);
  assert.deepEqual([...L.players], ['1']);
  assert.equal(L.mode, 'full');
  assert.equal(R.team, 'Bravo');
  assert.deepEqual([...R.audio], ['3']);
  assert.deepEqual([...R.reported], ['3']);
  assert.equal(R.format, 'wav');
  assert.deepEqual(R.marks, [{ start: 10, end: 12 }, { start: 30, end: 35 }]);
  assert.deepEqual([...R.reasons], ['reason.abuse']);
  assert.equal(R.reasonOther, 'spam');
  assert.equal(R.reportEdited, true);
  assert.equal(R.reportText, 'edited report');
  assert.equal(C.team, 'Bravo');
  assert.deepEqual(C.ranges, [{ start: 1, end: 2 }]);
  assert.equal(activeTab, 'report');
});

test('entries that no longer exist in the match are dropped', () => {
  setMatch(match());
  L.mode = 'full';
  R.format = 'wav';
  const ok = restoreState({
    v: 1,
    L: { team: 'Gone', periods: ['round:1:60000', 'round:99:999000'], players: ['1', '42'], mode: 'loud' },
    R: {
      team: 'Alpha', periods: ['round:1:60000'], audio: ['1', '3', '42'], reported: ['3'],
      format: 'ogg', marks: [{ start: 5, end: 1 }, { start: 700, end: 9999 }, { start: 2, end: 4 }, null],
      reasons: ['reason.abuse', 'reason.unknown'], reasonOther: 7,
    },
    C: { periods: ['round:77:77000'], ranges: 'nope' },
  });
  assert.equal(ok, true);
  assert.equal(L.team, 'Alpha'); // default kept
  assert.deepEqual([...L.periods], [1]);
  assert.deepEqual([...L.players], ['1']);
  assert.equal(L.mode, 'full'); // invalid mode keeps current value
  assert.deepEqual([...R.audio], ['1']); // player 3 is on the other team
  assert.deepEqual([...R.reported], []);
  assert.equal(R.format, 'wav');
  assert.deepEqual(R.marks, [{ start: 2, end: 4 }]);
  assert.deepEqual([...R.reasons], ['reason.abuse']);
  assert.equal(R.reasonOther, '');
  assert.deepEqual([...C.periods], [1]); // every saved period gone: default kept
  assert.deepEqual(C.ranges, []);
});

test('marks and cut ranges ending slightly past duration are clamped instead of dropped', () => {
  setMatch(match());
  restoreState({
    v: 1,
    R: { marks: [{ start: 590, end: 600.4 }, { start: 600, end: 605 }] },
    C: { ranges: [{ start: 595, end: 601.2 }, { start: 602, end: 610 }] },
  });
  assert.deepEqual(R.marks, [{ start: 590, end: 600 }]);
  assert.deepEqual(C.ranges, [{ start: 595, end: 600 }]);
});

test('saved periods follow stable identity when period IDs shift', () => {
  setMatch(match());
  L.periods = new Set([1]);
  R.periods = new Set([1]);
  const saved = JSON.parse(JSON.stringify(snapshotState()));

  // Insert an extra period at the start, shifting round 1 from id 1 to id 2.
  const shifted = match();
  shifted.periods = [
    { id: 0, kind: 'warmup', number: 0, start: 0, allTalk: true, talk: {} },
    { id: 1, kind: 'knife', number: 0, start: 30, allTalk: false, talk: {} },
    { id: 2, kind: 'round', number: 1, start: 60, allTalk: false, talk: { 1: 3 } },
    { id: 3, kind: 'break', number: 0, start: 300, allTalk: true, talk: {} },
  ];
  setMatch(shifted);
  restoreState(saved);
  assert.deepEqual([...L.periods], [2]);
  assert.deepEqual([...R.periods], [2]);
});

test('both teams only comes back when every saved period is all-talk', () => {
  setMatch(match());
  restoreState({ v: 1, R: { team: BOTH, periods: ['warmup:0:0', 'break:0:300000'], audio: ['1', '3'] } });
  assert.equal(R.team, BOTH);
  assert.deepEqual([...R.audio], ['1', '3']);

  setMatch(match());
  restoreState({ v: 1, R: { team: BOTH, periods: ['round:1:60000', 'break:0:300000'], audio: ['1', '3'] } });
  assert.equal(R.team, 'Alpha');
  assert.deepEqual([...R.audio], ['1']);
});

test('saves from another version or without a match are ignored', () => {
  setMatch(match());
  assert.equal(restoreState({ v: 999, R: { marks: [{ start: 1, end: 2 }] } }), false);
  assert.equal(R.marks.length, 0);
  assert.equal(restoreState(null), false);
  setMatch(null);
  assert.equal(restoreState({ v: 1 }), false);
});
