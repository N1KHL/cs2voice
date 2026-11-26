// Left-hand Listen panel on the Report tab (team/round/player preview only).
import { t } from './i18n.js';
import { M, colors, L, teamPlayers, talkIn, groupPlayers } from './state.js';
import { api } from './api.js';
import { $, $$, el, clock, debounce, formatError, periodRow, teamHead, renderTeamSwitch } from './ui.js';
import { makePlayer } from './player.js';

let leftPlayer = null;

export const refreshLeft = debounce(async () => {
  if (!M || !leftPlayer) return;
  const periods = [...L.periods].sort((a, b) => a - b);
  const players = [...L.players];
  if (!periods.length || !players.length) return leftPlayer.set(null, t('left.selectSome'));
  leftPlayer.loading();
  try {
    const pv = await api('Preview', {
      slot: 'left',
      periods,
      players,
      team: L.team,
      mode: L.mode,
      normalize: L.normalize,
    });
    leftPlayer.set(pv, pv.duration > 0 ? '' : t('left.nobody'));
  } catch (e) {
    leftPlayer.set(null, formatError(e));
  }
}, 250);

export function renderLeft() {
  if (!M) return;
  const root = $('#left');

  renderTeamSwitch($('#left-team'), M.teams, L.team, tm => {
    L.team = tm;
    renderLeft();
    refreshLeft();
  });

  const pl = $('.periods', root);
  pl.innerHTML = '';
  const tp = teamPlayers(L.team);
  for (const p of M.periods) {
    const talk = talkIn(p, p.allTalk ? M.players.map(x => x.id) : tp);
    pl.append(periodRow(p, L.periods.has(p.id), talk, on => {
      on ? L.periods.add(p.id) : L.periods.delete(p.id);
      refreshLeft();
    }));
  }

  const ul = $('.players', root);
  ul.innerHTML = '';
  for (const g of groupPlayers()) {
    ul.append(teamHead(
      g.team || t('common.spectators'),
      g.players.map(p => p.id),
      L.players,
      () => { renderLeft(); refreshLeft(); },
      g.team === L.team ? t('left.listening') : t('left.onlyAllTalk'),
    ));
    for (const p of g.players) {
      const other = g.team !== L.team;
      const r = el('div', 'row' + (L.players.has(p.id) ? ' on' : '') + (other ? ' dimmed' : ''));
      const cb = el('input');
      cb.type = 'checkbox';
      cb.checked = L.players.has(p.id);
      const dot = el('span', 'dot');
      dot.style.background = colors[p.id];
      r.append(cb, dot, el('span', 'pname', p.name));
      r.append(el('span', 'talk' + (p.talk > 0 ? ' has' : ''), p.talk > 0 ? clock(p.talk) : '—'));
      r.onclick = e => {
        if (e.target !== cb) cb.checked = !cb.checked;
        cb.checked ? L.players.add(p.id) : L.players.delete(p.id);
        r.classList.toggle('on', cb.checked);
        refreshLeft();
      };
      ul.append(r);
    }
  }

  $$('.col-head .links a', root).forEach(a => a.onclick = () => {
    const isPeriods = a.closest('.col').querySelector('.periods');
    const act = a.dataset.act;
    if (isPeriods) {
      if (act === 'all') M.periods.forEach(p => L.periods.add(p.id));
      if (act === 'none') L.periods.clear();
      if (act === 'talk') {
        L.periods.clear();
        M.periods.forEach(p => {
          if (talkIn(p, p.allTalk ? M.players.map(x => x.id) : tp) > 0) L.periods.add(p.id);
        });
      }
    } else {
      if (act === 'all') M.players.forEach(p => L.players.add(p.id));
      if (act === 'none') L.players.clear();
    }
    renderLeft();
    refreshLeft();
  });

  $$('.mode button', root).forEach(b => {
    b.classList.toggle('on', b.dataset.mode === L.mode);
    b.onclick = () => { L.mode = b.dataset.mode; renderLeft(); refreshLeft(); };
  });
  const norm = $('.normalize', root);
  norm.checked = L.normalize;
  norm.onchange = () => { L.normalize = norm.checked; refreshLeft(); };
}

export function initListenPanel() {
  leftPlayer = makePlayer($('#left .player'));
  leftPlayer.set(null, '');
}

export function setupListen() {
  renderLeft();
  refreshLeft();
}

export function redrawListen() {
  if (leftPlayer) leftPlayer.redraw();
}
