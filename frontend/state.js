// Shared match view and per-panel selection state (Listen, Export, Listen & Cut).
export const PALETTE = [
  '#ff7a1a', '#5aa0ff', '#4fd18b', '#e85d75', '#b48cff', '#ffd23f',
  '#3dd6d0', '#ff9ecf', '#a3d65c', '#c9a27e', '#8fb3ff', '#ff6b3d',
];

export const BOTH = '\u0000both'; // export team value: both teams (only for all-talk moments)

// Report reasons: the UI shows them translated, the report always gets the English text.
export const REASONS = [
  { key: 'reason.abuse', en: 'Verbal abuse / insults' },
  { key: 'reason.hate', en: 'Hate speech (racism, homophobia, sexism, ...)' },
  { key: 'reason.threats', en: 'Threats' },
  { key: 'reason.micspam', en: 'Mic spam / loud noises on purpose' },
  { key: 'reason.griefing', en: 'Intentionally ruining the game' },
];

function safeGet(key) {
  if (typeof window === 'undefined') return null;
  try { return localStorage.getItem(key); } catch { return null; }
}

function safeSet(key, val) {
  if (typeof window === 'undefined') return;
  try { localStorage.setItem(key, val); } catch {}
}

export let M = null;            // match view
export let colors = {};         // player id -> color
export let playerById = {};
export let periodById = {};
export let activeTab = safeGet('tab') === 'cut' ? 'cut' : 'report';

export const L = {
  id: 'left',
  team: '',
  periods: new Set(),
  players: new Set(),
  mode: 'compact',
  normalize: false,
};

export const R = {
  id: 'right',
  team: '',
  periods: new Set(),
  audio: new Set(),
  reported: new Set(),
  mode: 'compact',
  perRound: true,
  format: 'mp3',
  includeFull: true,
  includeIsolated: true,
  matchId: false,
  files: [],
  fileIdx: 0,
  reportEdited: false,
  reportText: '', // the report as edited by hand; used while reportEdited
  marks: [], // marked (toxic) moments, demo seconds [{start, end}], kept across files and modes
  reasons: new Set(),
  reasonOther: '',
};

export const C = {
  team: '',
  periods: new Set(),
  players: new Set(),
  mode: 'compact',
  normalize: false,
  format: 'mp3',
  bundle: safeGet('exportMode') === 'files' ? 'files' : 'zip',
  matchId: false,
  ranges: [],
};

export function setActiveTab(tab) {
  activeTab = tab === 'cut' ? 'cut' : 'report';
  safeSet('tab', activeTab);
  return activeTab;
}

/** Return IDs of players belonging to `team` in `match`. */
export function teamPlayers(team, match = M) {
  if (!match || !Array.isArray(match.players)) return [];
  return match.players.filter(p => p.team === team).map(p => p.id);
}

/** Sum talk seconds in `period` across player `ids`. */
export function talkIn(period, ids) {
  let sum = 0;
  for (const id of ids || []) {
    sum += (period && period.talk && period.talk[id]) || 0;
  }
  return sum;
}

/** Group `match.players` by `match.teams` in team order. */
export function groupPlayers(match = M) {
  if (!match) return [];
  const groups = [];
  for (const tm of match.teams || []) {
    groups.push({ team: tm, players: (match.players || []).filter(p => p.team === tm) });
  }
  return groups;
}

export function reasonText(reasons = R.reasons, other = R.reasonOther) {
  const parts = REASONS.filter(r => reasons.has(r.key)).map(r => r.en);
  if (other && other.trim()) parts.push(other.trim());
  return parts.join('; ');
}

/** Install a newly parsed match and reset panel selections to defaults. */
export function setMatch(match) {
  M = match;
  colors = {};
  playerById = {};
  periodById = {};

  if (!match) return;

  const players = match.players || [];
  const periods = match.periods || [];
  const teams = match.teams || [];

  players.forEach((p, i) => {
    colors[p.id] = PALETTE[i % PALETTE.length];
    playerById[p.id] = p;
  });
  periods.forEach(p => { periodById[p.id] = p; });

  const defaultTeam = teams[0] || '';
  const firstRound = periods.find(p => p.kind === 'round' && talkIn(p, teamPlayers(defaultTeam, match)) > 0) || periods[0];
  const defaultPeriods = firstRound ? [firstRound.id] : [];
  const allPlayerIds = players.map(p => p.id);

  L.team = defaultTeam;
  L.periods = new Set(defaultPeriods);
  L.players = new Set(allPlayerIds);

  R.team = defaultTeam;
  R.periods = new Set();
  R.audio = new Set();
  R.reported = new Set();
  R.files = [];
  R.fileIdx = 0;
  R.reportEdited = false;
  R.reportText = '';
  R.marks = [];
  R.reasons = new Set();
  R.reasonOther = '';

  C.team = defaultTeam;
  C.periods = new Set(defaultPeriods);
  C.players = new Set(allPlayerIds);
  C.ranges = [];
}

// ---------- saved sessions ----------

// SESSION_VERSION changes when the shape below does; older saves are ignored.
const SESSION_VERSION = 1;

/** Serializable copy of the selections, marks and report edits of all panels. */
export function snapshotState() {
  return {
    v: SESSION_VERSION,
    tab: activeTab,
    L: { team: L.team, periods: [...L.periods], players: [...L.players], mode: L.mode, normalize: L.normalize },
    R: {
      team: R.team, periods: [...R.periods], audio: [...R.audio], reported: [...R.reported],
      mode: R.mode, perRound: R.perRound, format: R.format,
      includeFull: R.includeFull, includeIsolated: R.includeIsolated, matchId: R.matchId,
      marks: R.marks.map(m => ({ start: m.start, end: m.end })),
      reasons: [...R.reasons], reasonOther: R.reasonOther,
      reportEdited: R.reportEdited, reportText: R.reportEdited ? R.reportText : '',
    },
    C: {
      team: C.team, periods: [...C.periods], players: [...C.players], mode: C.mode,
      normalize: C.normalize, format: C.format, matchId: C.matchId,
      ranges: C.ranges.map(m => ({ start: m.start, end: m.end })),
    },
  };
}

function spans(list) {
  const max = M && M.duration > 0 ? M.duration : Infinity;
  return (Array.isArray(list) ? list : [])
    .filter(s => s && Number.isFinite(s.start) && Number.isFinite(s.end) && s.start >= 0 && s.start < s.end && s.end <= max)
    .map(s => ({ start: s.start, end: s.end }))
    .sort((a, b) => a.start - b.start);
}

/**
 * Apply a saved session on top of the defaults set by setMatch. Anything that
 * no longer fits the match (a period or player gone after a parser change, an
 * unknown option) keeps its default, so a stale save can never break team
 * isolation: "both teams" survives only when every saved period is all-talk.
 * Returns false when the save is unusable.
 */
export function restoreState(saved) {
  if (!M || !saved || saved.v !== SESSION_VERSION) return false;
  const allTeams = M.teams || [];
  const teams = allTeams.filter(Boolean); // Export never picks spectators
  const team = (tm, fallback, list = allTeams) => (list.includes(tm) ? tm : fallback);
  // A saved list that lost every entry (e.g. periods renumbered) keeps the default.
  const ids = (list, known, fallback) => {
    const arr = Array.isArray(list) ? list : [];
    const kept = arr.filter(id => known[id]);
    return arr.length && !kept.length ? new Set(fallback) : new Set(kept);
  };
  const periods = (list, fallback = []) => ids(list, periodById, fallback);
  const players = (list, fallback = []) => ids(list, playerById, fallback);
  const mode = (m, fallback) => (m === 'compact' || m === 'full' ? m : fallback);
  const format = (f, fallback) => (f === 'mp3' || f === 'wav' ? f : fallback);
  const bool = (b, fallback) => (typeof b === 'boolean' ? b : fallback);
  const text = (s, fallback) => (typeof s === 'string' ? s : fallback);

  const l = saved.L || {};
  L.team = team(l.team, L.team);
  L.periods = periods(l.periods, L.periods);
  L.players = players(l.players, L.players);
  L.mode = mode(l.mode, L.mode);
  L.normalize = bool(l.normalize, L.normalize);

  const r = saved.R || {};
  R.periods = periods(r.periods);
  const allTalkOnly = R.periods.size > 0 && [...R.periods].every(id => periodById[id].allTalk);
  R.team = r.team === BOTH && allTalkOnly ? BOTH : team(r.team, R.team, teams);
  const rightTeams = R.team === BOTH ? teams : [R.team];
  const onRightTeam = id => rightTeams.includes(playerById[id].team);
  R.audio = new Set([...players(r.audio)].filter(onRightTeam));
  R.reported = new Set([...players(r.reported)].filter(onRightTeam));
  R.mode = mode(r.mode, R.mode);
  R.perRound = bool(r.perRound, R.perRound);
  R.format = format(r.format, R.format);
  R.includeFull = bool(r.includeFull, R.includeFull);
  R.includeIsolated = bool(r.includeIsolated, R.includeIsolated);
  R.matchId = bool(r.matchId, R.matchId);
  R.marks = spans(r.marks);
  R.reasons = new Set((Array.isArray(r.reasons) ? r.reasons : []).filter(k => REASONS.some(x => x.key === k)));
  R.reasonOther = text(r.reasonOther, '');
  R.reportText = text(r.reportText, '');
  R.reportEdited = r.reportEdited === true && R.reportText !== '';

  const c = saved.C || {};
  C.team = team(c.team, C.team);
  C.periods = periods(c.periods, C.periods);
  C.players = players(c.players, C.players);
  C.mode = mode(c.mode, C.mode);
  C.normalize = bool(c.normalize, C.normalize);
  C.format = format(c.format, C.format);
  C.matchId = bool(c.matchId, C.matchId);
  C.ranges = spans(c.ranges);

  if (saved.tab === 'cut' || saved.tab === 'report') setActiveTab(saved.tab);
  return true;
}
