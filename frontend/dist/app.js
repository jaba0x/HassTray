// HassTray popover. Plain JavaScript, no build step. The page asks Go for a
// snapshot about once a second and redraws only when something changed.
(function () {
  'use strict';

  var BASE = window.HASSTRAY_ASSETS || '';
  var root = document.getElementById('app');

  var THEMES = [
    { id: 'glass', name: 'Liquid Glass', note: 'Follows light and dark mode', bg: 'linear-gradient(150deg,#cfe6ff,#eef2f9 55%,#dcd9ff)', card: 'rgba(255,255,255,.8)', accent: 'linear-gradient(160deg,#2b95ff,#0068d6 60%,#4f4ddb)' },
    { id: 'midnight', name: 'Midnight Mint', note: 'Dark ink, mint accent', bg: '#0a0e17', card: 'rgba(255,255,255,.1)', accent: '#4fe3c1' },
    { id: 'neon', name: 'Neon Terminal', note: 'Black, lime, monospace', bg: '#000', card: '#1a1f10', accent: '#c6ff3d' },
    { id: 'sunset', name: 'Sunset', note: 'Plum with warm colours', bg: 'linear-gradient(140deg,#5a1d3c,#1a1024 55%,#3b2470)', card: 'rgba(255,255,255,.14)', accent: 'linear-gradient(140deg,#ffc465,#ff8a6b)' }
  ];

  var ICONS = {
    light: 'M9 18h6M10 21h4M12 3a6 6 0 0 0-4 10.5c.6.6 1 1.4 1 2.5h6c0-1.1.4-1.9 1-2.5A6 6 0 0 0 12 3z',
    switch: 'M12 3v9M6.3 7.3a8 8 0 1 0 11.4 0',
    input_boolean: 'M12 3v9M6.3 7.3a8 8 0 1 0 11.4 0',
    fan: 'M12 12c0-5 1-8 4-8s2 6-4 8zM12 12c5 0 8 1 8 4s-6 2-8-4zM12 12c0 5-1 8-4 8s-2-6 4-8zM12 12c-5 0-8-1-8-4s6-2 8 4z',
    cover: 'M4 6h16M4 10h16M4 14h16M6 6v14M18 6v14',
    climate: 'M14 14.8V5a2 2 0 0 0-4 0v9.8a4 4 0 1 0 4 0z',
    lock: 'M6 11h12v9H6zM8.5 11V8a3.5 3.5 0 0 1 7 0v3',
    media_player: 'M7 5l12 7-12 7z',
    scene: 'M12 3l2.2 5.6L20 9.3l-4.4 3.9L17 19l-5-3-5 3 1.4-5.8L4 9.3l5.8-.7z',
    script: 'M7 5l12 7-12 7z',
    automation: 'M13 2L4 14h7l-1 8 9-12h-7z',
    button: 'M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8zM12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z',
    input_button: 'M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8zM12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z',
    vacuum: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6z',
    fallback: 'M3 12h4l3-8 4 16 3-8h4'
  };
  var UI = {
    settings: 'M4 7h10M18 7h2M4 17h2M10 17h10M16 5a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM8 15a2 2 0 1 0 0 4 2 2 0 0 0 0-4z',
    back: 'M15 5l-7 7 7 7',
    check: 'M5 12.5l5 4.5 9-10',
    up: 'M6 15l6-6 6 6',
    down: 'M6 9l6 6 6-6',
    close: 'M6 6l12 12M18 6L6 18',
    search: 'M11 4.5a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13zM16 16l4 4',
    graph: 'M3 17l6-7 4 4 8-9',
    out: 'M14 5h5v5M19 5l-8 8M11 6H6v12h12v-5'
  };

  var S = {
    snap: null,
    view: 'main',          // main | detail | settings
    tab: 'entities',       // entities | look | connection | about
    detail: { id: '', hours: 24, data: null, error: '' },
    picker: { all: [], q: '', domain: 'all' },
    form: { url: '', token: '', busy: false, error: '' },
    busy: {},
    flash: ''
  };

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }
  function icon(path, cls) {
    return '<svg class="ic ' + (cls || '') + '" viewBox="0 0 24 24" aria-hidden="true"><path d="' + path + '"/></svg>';
  }
  function mark() {
    return '<span class="logo" aria-hidden="true"><svg viewBox="0 0 64 64">' +
      '<path class="house" d="M32 9L55 28V49a7 7 0 0 1-7 7H16a7 7 0 0 1-7-7V28Z"/>' +
      '<path class="pulse" d="M17 41h7.5l4-9 6 14 4-7h8.5"/></svg></span>';
  }

  function api(path, body) {
    var opts = body === undefined ? {} : {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    };
    return fetch(path, opts).then(function (r) {
      return r.json().then(function (j) {
        if (!r.ok) throw new Error((j && j.error) || 'Something went wrong');
        return j;
      });
    });
  }

  // ---------- numbers ----------

  function fmt(v) {
    var abs = Math.abs(v), p = 1;
    if (abs >= 1000) p = 0; else if (abs < 10 && abs !== Math.trunc(abs)) p = 2;
    var s = v.toFixed(p);
    if (s.indexOf('.') >= 0) s = s.replace(/0+$/, '').replace(/\.$/, '');
    return s === '-0' ? '0' : s;
  }
  function withUnit(v, unit) {
    if (!unit) return fmt(v);
    return unit === '%' ? fmt(v) + '%' : fmt(v) + ' ' + unit;
  }

  // ---------- small graphs ----------

  function linePath(vals, w, h, pad) {
    var min = Math.min.apply(null, vals), max = Math.max.apply(null, vals);
    if (max - min < 1e-9) { min -= 1; max += 1; }
    var step = w / (vals.length - 1);
    var pts = vals.map(function (v, i) {
      var y = pad + (h - 2 * pad) * (1 - (v - min) / (max - min));
      return [i * step, y];
    });
    var d = pts.map(function (p, i) { return (i ? 'L' : 'M') + p[0].toFixed(1) + ' ' + p[1].toFixed(1); }).join(' ');
    return { d: d, pts: pts, min: min, max: max };
  }
  function spark(vals) {
    if (!vals || vals.length < 2) return '';
    var p = linePath(vals, 118, 32, 3);
    return '<svg class="spark" viewBox="0 0 118 32" aria-hidden="true">' +
      '<path class="area" d="' + p.d + ' L118 32 L0 32 Z"/><path class="stroke" d="' + p.d + '"/></svg>';
  }

  // ---------- main view ----------

  function viewMain() {
    var s = S.snap;
    if (!s.configured) return viewSetup();
    var status = s.connected ? '<span class="dot ok"></span>Connected'
      : (s.error ? '<span class="dot bad"></span>Offline' : '<span class="dot"></span>Connecting');
    var html = '<div class="view">' +
      '<div class="head">' + mark() +
      '<div class="grow"><h1>' + esc(s.name || 'Home') + '</h1><div class="status">' + status + '</div></div>' +
      '<button class="iconbtn" data-go="settings" aria-label="Settings">' + icon(UI.settings) + '</button></div>';
    if (s.update && s.update.available) {
      html += '<button class="notice" data-open="release"><span>HassTray ' + esc(s.update.latest) + ' is out</span><span>Get it</span></button>';
    }
    if (!s.connected && s.error) html += '<div class="errorline">' + esc(s.error) + '</div>';
    if (S.flash) html += '<div class="errorline">' + esc(S.flash) + '</div>';

    var controls = s.entities.filter(function (e) { return e.kind !== 'sensor'; });
    var sensors = s.entities.filter(function (e) { return e.kind === 'sensor'; });
    html += '<div class="scroll">';
    if (!s.entities.length) {
      html += '<div class="empty"><h2>Nothing in the tray yet</h2>' +
        '<p>Pick the lights, switches and sensors you want here. ' + (s.total ? s.total + ' entities are waiting.' : '') + '</p>' +
        '<button class="btn primary" data-go="settings">Choose entities</button></div>';
    }
    if (controls.length) {
      html += '<section><div class="label">Controls</div><div class="tiles">' + controls.map(tile).join('') + '</div></section>';
    }
    if (sensors.length) {
      html += '<section><div class="label">Sensors</div><div class="rows">' + sensors.map(row).join('') + '</div></section>';
    }
    html += '</div><div class="foot">' +
      '<button class="credit" data-open="clihero">Built by clihero.com</button><span class="spacer"></span>' +
      '<button class="btn" data-open="ha">Open Home Assistant ' + icon(UI.out) + '</button></div></div>';
    return html;
  }

  function tile(e) {
    var cls = 'tile' + (e.on ? ' on' : '') + (e.available ? '' : ' off-line') + (S.busy[e.id] ? ' busy' : '');
    return '<button class="' + cls + '" data-act="' + esc(e.id) + '" aria-pressed="' + (e.on ? 'true' : 'false') + '"' +
      (e.available ? '' : ' disabled') + '>' +
      '<span class="badge">' + icon(ICONS[e.domain] || ICONS.fallback) + '</span>' +
      '<span><span class="name" style="display:block">' + esc(e.name) + '</span><span class="sub" style="display:block">' + esc(e.detail) + '</span></span></button>';
  }

  function row(e) {
    var inner = '<span class="text"><span class="name" style="display:block">' + esc(e.name) + '</span>' +
      '<span class="value num">' + esc(e.value) + '</span></span>' + (e.graph ? spark(e.spark) : '');
    var cls = 'row' + (e.available ? '' : ' off-line');
    if (e.numeric) return '<button class="' + cls + '" data-detail="' + esc(e.id) + '">' + inner + '</button>';
    return '<div class="' + cls + '">' + inner + '</div>';
  }

  // ---------- first run ----------

  function connectForm(saved) {
    var f = S.form;
    return '<div class="field"><label for="f-url">Home Assistant address</label>' +
      '<input id="f-url" type="text" spellcheck="false" autocomplete="off" placeholder="http://homeassistant.local:8123" value="' + esc(f.url) + '"></div>' +
      '<div class="field"><label for="f-token">Long-lived access token</label>' +
      '<input id="f-token" type="password" spellcheck="false" autocomplete="off" placeholder="' + (saved ? 'Saved in your Keychain. Leave empty to keep it.' : 'Paste the token') + '" value="' + esc(f.token) + '">' +
      '<small>Create one in Home Assistant under your profile, on the Security tab. It is stored in the macOS Keychain.</small></div>' +
      (f.error ? '<div class="errorline" style="margin:0">' + esc(f.error) + '</div>' : '') +
      '<button class="btn primary" data-connect' + (f.busy ? ' disabled' : '') + '>' + (f.busy ? 'Connecting…' : (saved ? 'Save and reconnect' : 'Connect')) + '</button>';
  }

  function viewSetup() {
    return '<div class="view"><div class="head">' + mark() + '<div class="grow"><h1>HassTray</h1>' +
      '<div class="status">Home Assistant in your menu bar</div></div>' +
      '<button class="iconbtn" data-go="settings" aria-label="Settings">' + icon(UI.settings) + '</button></div>' +
      '<div class="scroll"><div class="empty" style="margin:0;padding:18px 12px 4px"><h2>Connect your home</h2>' +
      '<p>HassTray talks straight to your Home Assistant. Nothing goes through anyone else\'s servers.</p></div>' +
      connectForm(false) + '</div>' +
      '<div class="foot"><button class="credit" data-open="clihero">Built by clihero.com</button></div></div>';
  }

  // ---------- graph view ----------

  function viewDetail() {
    var d = S.detail, data = d.data;
    var e = (S.snap.entities || []).filter(function (x) { return x.id === d.id; })[0];
    var name = data ? data.name : (e ? e.name : d.id);
    var value = e ? e.value : (data ? data.value : '');
    var html = '<div class="view"><div class="head">' +
      '<button class="iconbtn" data-go="main" aria-label="Back">' + icon(UI.back) + '</button>' +
      '<div class="grow"><h1>' + esc(name) + '</h1><div class="status">' + esc(d.id) + '</div></div></div>' +
      '<div class="scroll"><div style="display:flex;align-items:flex-end;justify-content:space-between;gap:10px">' +
      '<div class="big num">' + esc(value) + '</div><div class="seg">' +
      [[1, '1H'], [24, '24H'], [168, '7D']].map(function (r) {
        return '<button data-hours="' + r[0] + '" aria-pressed="' + (d.hours === r[0] ? 'true' : 'false') + '">' + r[1] + '</button>';
      }).join('') + '</div></div>';
    if (d.error) {
      html += '<div class="errorline" style="margin:0">' + esc(d.error) + '</div>';
    } else if (!data) {
      html += '<div class="chartbox"><div class="muted" style="height:190px;display:grid;place-items:center">Loading…</div></div>';
    } else if (!data.values || data.values.length < 2) {
      html += '<div class="chartbox"><div class="muted" style="height:190px;display:grid;place-items:center;text-align:center">No readings in this period.</div></div>';
    } else {
      html += '<div class="chartbox" id="chartbox">' + chart(data) + '<div class="tip hidden" id="tip"></div></div>' +
        '<div class="stats">' +
        '<div class="stat"><span>Lowest</span><b class="num">' + esc(withUnit(data.stats.min, data.unit)) + '</b></div>' +
        '<div class="stat"><span>Average</span><b class="num">' + esc(withUnit(data.stats.avg, data.unit)) + '</b></div>' +
        '<div class="stat"><span>Highest</span><b class="num">' + esc(withUnit(data.stats.max, data.unit)) + '</b></div></div>';
    }
    return html + '</div></div>';
  }

  var CH = { w: 352, top: 12, bottom: 162, h: 190 };

  function timeLabel(ms, hours) {
    var t = new Date(ms);
    var hm = ('0' + t.getHours()).slice(-2) + ':' + ('0' + t.getMinutes()).slice(-2);
    if (hours <= 24) return hm;
    return ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'][t.getDay()] + ' ' + hm;
  }

  function chart(data) {
    var p = linePath(data.values, CH.w, CH.bottom + CH.top, CH.top);
    var svg = '<svg class="chart" viewBox="0 0 ' + CH.w + ' ' + CH.h + '" role="img" aria-label="' + esc(data.name) + ' over the selected period">';
    [0, 0.5, 1].forEach(function (f) {
      var y = CH.top + (CH.bottom - CH.top) * f;
      svg += '<path class="grid" d="M0 ' + y + ' H' + CH.w + '"/>';
    });
    svg += '<path class="area" d="' + p.d + ' L' + CH.w + ' ' + CH.bottom + ' L0 ' + CH.bottom + ' Z"/>' +
      '<path class="stroke" d="' + p.d + '"/>' +
      '<path class="cursor hidden" id="cursor" d="M0 ' + CH.top + ' V' + CH.bottom + '"/>' +
      '<circle class="knob hidden" id="knob" r="4.5" cx="0" cy="0"/>' +
      '<text x="0" y="182">' + esc(timeLabel(data.from, data.hours)) + '</text>' +
      '<text x="' + CH.w / 2 + '" y="182" text-anchor="middle">' + esc(timeLabel((data.from + data.to) / 2, data.hours)) + '</text>' +
      '<text x="' + CH.w + '" y="182" text-anchor="end">Now</text>' +
      '<text x="' + CH.w + '" y="' + (CH.top - 3) + '" text-anchor="end">' + esc(withUnit(p.max, data.unit)) + '</text>' +
      '<text x="' + CH.w + '" y="' + (CH.bottom - 4) + '" text-anchor="end">' + esc(withUnit(p.min, data.unit)) + '</text></svg>';
    chart.pts = p.pts;
    return svg;
  }

  function hoverChart(ev) {
    var box = document.getElementById('chartbox');
    var data = S.detail.data;
    if (!box || !data || !chart.pts) return;
    var svg = box.querySelector('svg'), r = svg.getBoundingClientRect();
    var n = data.values.length;
    var i = Math.round((ev.clientX - r.left) / r.width * (n - 1));
    i = Math.max(0, Math.min(n - 1, i));
    var pt = chart.pts[i];
    var cursor = document.getElementById('cursor'), knob = document.getElementById('knob'), tip = document.getElementById('tip');
    cursor.setAttribute('d', 'M' + pt[0] + ' ' + CH.top + ' V' + CH.bottom);
    knob.setAttribute('cx', pt[0]); knob.setAttribute('cy', pt[1]);
    cursor.classList.remove('hidden'); knob.classList.remove('hidden'); tip.classList.remove('hidden');
    var when = data.from + (data.to - data.from) * (i + 0.5) / n;
    tip.textContent = timeLabel(when, data.hours) + ' · ' + withUnit(data.values[i], data.unit);
    var x = (svg.getBoundingClientRect().left - box.getBoundingClientRect().left) + pt[0] / CH.w * r.width;
    tip.style.left = Math.max(60, Math.min(box.clientWidth - 60, x)) + 'px';
  }
  function leaveChart() {
    ['cursor', 'knob', 'tip'].forEach(function (id) {
      var el = document.getElementById(id);
      if (el) el.classList.add('hidden');
    });
  }

  function loadHistory() {
    var d = S.detail;
    d.data = null; d.error = '';
    render();
    var id = d.id, hours = d.hours;
    api('/api/history?id=' + encodeURIComponent(id) + '&hours=' + hours).then(function (data) {
      if (S.detail.id !== id || S.detail.hours !== hours) return;
      d.data = data; render();
    }).catch(function (err) {
      if (S.detail.id !== id || S.detail.hours !== hours) return;
      d.error = err.message; render();
    });
  }

  // ---------- settings ----------

  function viewSettings() {
    var tabs = [['entities', 'Entities'], ['look', 'Themes'], ['connection', 'Connection'], ['about', 'About']];
    var html = '<div class="view"><div class="head">' +
      '<button class="iconbtn" data-go="main" aria-label="Back">' + icon(UI.back) + '</button>' +
      '<div class="grow"><h1>Settings</h1></div></div>' +
      '<div class="tabs" role="tablist">' + tabs.map(function (t) {
        return '<button role="tab" data-tab="' + t[0] + '" aria-selected="' + (S.tab === t[0] ? 'true' : 'false') + '">' + t[1] + '</button>';
      }).join('') + '</div>';
    if (S.tab === 'entities') html += tabEntities();
    if (S.tab === 'look') html += tabLook();
    if (S.tab === 'connection') html += tabConnection();
    if (S.tab === 'about') html += tabAbout();
    return html + '</div>';
  }

  function tabEntities() {
    if (!S.snap.configured) {
      return '<div class="scroll"><div class="empty"><h2>Not connected yet</h2><p>Add your Home Assistant first, then pick entities here.</p>' +
        '<button class="btn primary" data-tab="connection">Set up connection</button></div></div>';
    }
    return '<div class="scroll">' +
      '<section id="chosen">' + chosenList() + '</section>' +
      '<section><div class="label">All entities<span id="count"></span></div>' +
      '<div class="search" style="margin-bottom:8px">' + icon(UI.search) +
      '<input id="q" type="text" spellcheck="false" autocomplete="off" aria-label="Search entities" placeholder="Search by name or entity ID" value="' + esc(S.picker.q) + '"></div>' +
      '<div class="chips" id="domains" style="margin-bottom:8px"></div>' +
      '<div class="list" id="picklist"></div></section></div>';
  }

  function chosenList() {
    var s = S.snap, list = s.entities;
    var html = '<div class="label">In the tray<span>Menu bar widgets ' + s.widgets + ' of ' + s.maxWidgets + '</span></div>';
    if (S.flash) html += '<div class="errorline" style="margin:0 0 8px">' + esc(S.flash) + '</div>';
    if (!list.length) return html + '<div class="muted" style="padding:2px">Tick entities below to add them.</div>';
    html += '<div class="list">' + list.map(function (e, i) {
      var full = !e.widget && s.widgets >= s.maxWidgets;
      return '<div class="item"><div class="text"><div class="name">' + esc(e.name) + '</div><div class="id">' + esc(e.id) + '</div></div>' +
        '<div class="acts">' +
        (e.numeric || e.graph ? '<button class="chip" data-graph="' + esc(e.id) + '" aria-pressed="' + (e.graph ? 'true' : 'false') + '" title="Show a graph for this entity">' + icon(UI.graph) + 'Graph</button>' : '') +
        '<button class="chip" data-widget="' + esc(e.id) + '" aria-pressed="' + (e.widget ? 'true' : 'false') + '"' + (full ? ' disabled' : '') +
        ' title="' + (full ? 'The menu bar holds up to ' + s.maxWidgets + ' widgets' : 'Show the value in the menu bar') + '">Menu bar</button>' +
        '<button class="mini" data-move="' + esc(e.id) + '" data-delta="-1" aria-label="Move ' + esc(e.name) + ' up"' + (i === 0 ? ' disabled' : '') + '>' + icon(UI.up) + '</button>' +
        '<button class="mini" data-move="' + esc(e.id) + '" data-delta="1" aria-label="Move ' + esc(e.name) + ' down"' + (i === list.length - 1 ? ' disabled' : '') + '>' + icon(UI.down) + '</button>' +
        '<button class="mini" data-select="' + esc(e.id) + '" data-on="0" aria-label="Remove ' + esc(e.name) + '">' + icon(UI.close) + '</button>' +
        '</div></div>';
    }).join('') + '</div>';
    return html;
  }

  function drawPicker() {
    var list = document.getElementById('picklist');
    if (!list) return;
    var p = S.picker, q = p.q.trim().toLowerCase();
    var counts = {};
    p.all.forEach(function (e) { counts[e.domain] = (counts[e.domain] || 0) + 1; });
    var domains = Object.keys(counts).sort(function (a, b) { return counts[b] - counts[a]; });
    if (p.domain !== 'all' && !counts[p.domain]) p.domain = 'all';
    document.getElementById('domains').innerHTML = ['all'].concat(domains).map(function (d) {
      return '<button class="chip" data-domain="' + esc(d) + '" aria-pressed="' + (p.domain === d ? 'true' : 'false') + '">' +
        esc(d === 'all' ? 'All' : d.replace(/_/g, ' ')) + '</button>';
    }).join('');
    var hits = p.all.filter(function (e) {
      if (p.domain !== 'all' && e.domain !== p.domain) return false;
      return !q || e.name.toLowerCase().indexOf(q) >= 0 || e.id.toLowerCase().indexOf(q) >= 0;
    });
    var shown = hits.slice(0, 120);
    document.getElementById('count').textContent = hits.length + (hits.length === 1 ? ' entity' : ' entities');
    list.innerHTML = shown.map(function (e) {
      return '<div class="item"><button class="check" data-select="' + esc(e.id) + '" data-on="' + (e.selected ? '0' : '1') + '" aria-pressed="' + (e.selected ? 'true' : 'false') +
        '" aria-label="Show ' + esc(e.name) + ' in the tray">' + icon(UI.check) + '</button>' +
        '<div class="text"><div class="name">' + esc(e.name) + '</div><div class="id">' + esc(e.id) + '</div></div>' +
        '<div class="acts muted num" style="font-size:11px;max-width:96px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">' + esc(e.value) + '</div></div>';
    }).join('') + (hits.length > shown.length ? '<div class="muted" style="padding:6px 2px">Showing the first ' + shown.length + '. Search to narrow it down.</div>' : '') +
      (!hits.length ? '<div class="muted" style="padding:6px 2px">' + (p.all.length ? 'Nothing matches.' : 'Waiting for Home Assistant…') + '</div>' : '');
  }

  function loadPicker() {
    return api('/api/entities').then(function (all) {
      S.picker.all = all;
      drawPicker();
    }).catch(function () {});
  }

  function tabLook() {
    return '<div class="scroll"><section><div class="label">Theme</div><div class="themes">' + THEMES.map(function (t) {
      return '<button class="theme" data-theme-pick="' + t.id + '" aria-pressed="' + (S.snap.theme === t.id ? 'true' : 'false') + '">' +
        '<span class="swatch" style="background:' + t.bg + '"><i style="background:' + t.card + ';width:70%"></i><i style="background:' + t.accent + ';width:45%"></i></span>' +
        '<span><b>' + t.name + '</b><small>' + t.note + '</small></span></button>';
    }).join('') + '</div></section></div>';
  }

  function tabConnection() {
    var s = S.snap;
    var html = '<div class="scroll">';
    if (s.configured) {
      html += '<div class="row"><span class="text"><span class="name" style="display:block">' + esc(s.url) + '</span>' +
        '<span class="value" style="font-size:14px">' + (s.connected ? 'Connected' : 'Not connected') +
        (s.haVersion ? ' · Home Assistant ' + esc(s.haVersion) : '') + '</span></span></div>';
      if (!s.connected && s.error) html += '<div class="errorline" style="margin:0">' + esc(s.error) + '</div>';
    }
    html += connectForm(s.configured);
    if (s.configured) {
      html += '<button class="btn danger" data-disconnect>Disconnect and forget the token</button>';
    }
    return html + '</div>';
  }

  function tabAbout() {
    return '<div class="scroll about">' + mark().replace('class="logo"', 'class="logo mark"') +
      '<h2>HassTray</h2><p>Version ' + esc(S.snap.version) + '. Home Assistant in your Mac menu bar: the entities you pick, their graphs, and up to five values next to the icon.</p>' +
      '<button class="by" data-open="clihero"><img src="' + BASE + 'clihero-logo.png" alt="clihero.com">' +
      '<span>Built by <b>Jaba Macharashvili</b> · clihero.com</span></button>' +
      '<div class="rowbtns"><button class="btn wide" data-open="github">Source on GitHub ' + icon(UI.out) + '</button>' +
      '<button class="btn wide" data-open="release">Releases ' + icon(UI.out) + '</button></div>' +
      '<p style="font-size:11px">An independent project. Not affiliated with Home Assistant or the Open Home Foundation.</p>' +
      '<span class="spacer"></span><button class="btn danger" data-quit>Quit HassTray</button></div>';
  }

  // ---------- drawing ----------

  function render() {
    if (!S.snap) return;
    root.setAttribute('data-theme', S.snap.theme || 'glass');
    var keepScroll = (root.querySelector('.scroll') || {}).scrollTop || 0;
    root.innerHTML = S.view === 'detail' ? viewDetail() : S.view === 'settings' ? viewSettings() : viewMain();
    var sc = root.querySelector('.scroll');
    if (sc) sc.scrollTop = keepScroll;
    if (S.view === 'settings' && S.tab === 'entities') drawPicker();
    var box = document.getElementById('chartbox');
    if (box) {
      box.addEventListener('mousemove', hoverChart);
      box.addEventListener('mouseleave', leaveChart);
    }
  }

  function typing() {
    var el = document.activeElement;
    return el && el.tagName === 'INPUT';
  }

  function onSnap(snap) {
    var first = !S.snap, changed = first || snap.rev !== S.snap.rev;
    S.snap = snap;
    if (first) S.form.url = snap.url || '';
    if (!changed) return;
    if (S.view === 'settings' && S.tab === 'entities' && document.getElementById('chosen')) {
      // keep the search box and the scroll position; only refresh the lists
      root.setAttribute('data-theme', snap.theme);
      document.getElementById('chosen').innerHTML = chosenList();
      loadPicker();
      return;
    }
    if (typing()) return;
    render();
  }

  function poll() {
    return api('/api/snapshot').then(onSnap).catch(function () {});
  }
  function soon() {
    setTimeout(poll, 150);
    setTimeout(poll, 900);
  }

  function go(view) {
    S.view = view; S.flash = '';
    render();
    var sc = root.querySelector('.scroll');
    if (sc) sc.scrollTop = 0;
    if (view === 'settings' && S.tab === 'entities') loadPicker();
  }

  function post(path, body) {
    S.flash = '';
    return api(path, body).then(poll).catch(function (err) {
      S.flash = err.message;
      if (document.getElementById('chosen')) document.getElementById('chosen').innerHTML = chosenList(); else render();
    });
  }

  function connect() {
    var f = S.form;
    var u = document.getElementById('f-url'), t = document.getElementById('f-token');
    if (u) f.url = u.value;
    if (t) f.token = t.value;
    f.busy = true; f.error = '';
    render();
    api('/api/connect', { url: f.url, token: f.token }).then(function () {
      f.busy = false; f.token = '';
      if (S.view === 'settings') { S.tab = 'entities'; }
      return poll();
    }).then(function () {
      S.form.url = S.snap.url;
      if (S.view === 'settings') go('settings'); else { S.view = 'settings'; S.tab = 'entities'; go('settings'); }
    }).catch(function (err) {
      f.busy = false; f.error = err.message;
      render();
    });
  }

  root.addEventListener('click', function (ev) {
    var el = ev.target.closest('button');
    if (!el || el.disabled) return;
    var d = el.dataset;
    if (d.go) return go(d.go);
    if (d.tab) { S.tab = d.tab; S.flash = ''; S.form.error = ''; return go('settings'); }
    if (d.open) return void api('/api/open', { target: d.open }).catch(function () {});
    if (d.act) {
      S.busy[d.act] = true; S.flash = ''; render();
      return void api('/api/act', { id: d.act }).catch(function (err) { S.flash = err.message; })
        .then(function () { delete S.busy[d.act]; render(); soon(); });
    }
    if (d.detail) { S.detail = { id: d.detail, hours: 24, data: null, error: '' }; S.view = 'detail'; return loadHistory(); }
    if (d.hours) { S.detail.hours = Number(d.hours); return loadHistory(); }
    if (d.select) return void post('/api/select', { id: d.select, on: d.on === '1' });
    if (d.graph) return void post('/api/graph', { id: d.graph, on: el.getAttribute('aria-pressed') !== 'true' });
    if (d.widget) return void post('/api/widget', { id: d.widget, on: el.getAttribute('aria-pressed') !== 'true' });
    if (d.move) return void post('/api/move', { id: d.move, delta: Number(d.delta) });
    if (d.domain) { S.picker.domain = d.domain; return drawPicker(); }
    if (d.themePick) {
      S.snap.theme = d.themePick; render();
      return void api('/api/theme', { theme: d.themePick }).then(poll).catch(function () {});
    }
    if ('connect' in d) return connect();
    if ('disconnect' in d) {
      return void api('/api/disconnect', {}).then(function () { S.form = { url: '', token: '', busy: false, error: '' }; return poll(); })
        .then(function () { render(); }).catch(function () {});
    }
    if ('quit' in d) return void api('/api/quit', {}).catch(function () {});
  });

  root.addEventListener('input', function (ev) {
    if (ev.target.id === 'q') { S.picker.q = ev.target.value; drawPicker(); }
    if (ev.target.id === 'f-url') S.form.url = ev.target.value;
    if (ev.target.id === 'f-token') S.form.token = ev.target.value;
  });

  document.addEventListener('keydown', function (ev) {
    if (ev.key === 'Enter' && (ev.target.id === 'f-url' || ev.target.id === 'f-token')) return connect();
    if (ev.key !== 'Escape') return;
    if (S.view !== 'main') { ev.preventDefault(); ev.stopPropagation(); return go('main'); }
    api('/api/hide', {}).catch(function () {});
  }, true);

  poll().then(function () { render(); });
  setInterval(poll, 1200);
})();
