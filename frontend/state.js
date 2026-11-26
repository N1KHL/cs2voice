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
  R.marks = [];
  R.reasons = new Set();
  R.reasonOther = '';

  C.team = defaultTeam;
  C.periods = new Set(defaultPeriods);
  C.players = new Set(allPlayerIds);
  C.ranges = [];
}
