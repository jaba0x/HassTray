// Sample data for working on the interface in a normal browser. It answers
// the same /api routes the Go side does, so app.js runs unchanged.
(function () {
  function wave(n, base, amp, seed) {
    var out = [];
    for (var i = 0; i < n; i++) {
      out.push(base + amp * Math.sin(i / n * Math.PI * 2 + seed) + amp * 0.25 * Math.sin(i * 1.7 + seed));
    }
    return out;
  }
  function bell(n, peak) {
    var out = [];
    for (var i = 0; i < n; i++) {
      var x = (i / (n - 1) - 0.55) / 0.17;
      out.push(Math.max(0, peak * Math.exp(-x * x)));
    }
    return out;
  }
  var params = new URLSearchParams(location.search);
  var db = {
    rev: 1,
    configured: !params.has('setup'),
    theme: params.get('theme') || 'glass',
    widgets: ['sensor.outdoor_temperature', 'sensor.solar_power'],
    graphs: ['sensor.outdoor_temperature', 'sensor.solar_power', 'sensor.home_power'],
    order: ['light.living_room', 'light.kitchen_table', 'cover.pergola_louvers', 'climate.air_conditioner',
      'sensor.outdoor_temperature', 'sensor.solar_power', 'sensor.home_power', 'sensor.living_room_humidity', 'binary_sensor.front_door'],
    all: {
      'light.living_room': { name: 'Living room', kind: 'toggle', on: true, detail: 'On · 80%', value: 'On' },
      'light.kitchen_table': { name: 'Kitchen table', kind: 'toggle', on: false, detail: 'Off', value: 'Off' },
      'cover.pergola_louvers': { name: 'Pergola louvers', kind: 'toggle', on: false, detail: 'Closed', value: 'Closed' },
      'climate.air_conditioner': { name: 'Air conditioner', kind: 'toggle', on: true, detail: 'Cooling · 22°', value: 'Cool' },
      'scene.movie_night': { name: 'Movie night', kind: 'action', on: false, detail: 'Tap to run', value: 'Scene' },
      'lock.front_door': { name: 'Front door lock', kind: 'toggle', on: false, detail: 'Locked', value: 'Locked' },
      'sensor.outdoor_temperature': { name: 'Outdoor temperature', kind: 'sensor', value: '12.4 °C', numeric: true, unit: '°C', spark: wave(48, 12, 4, 1) },
      'sensor.solar_power': { name: 'Solar production', kind: 'sensor', value: '3.2 kW', numeric: true, unit: 'kW', spark: bell(48, 4.1) },
      'sensor.home_power': { name: 'Home power', kind: 'sensor', value: '1.1 kW', numeric: true, unit: 'kW', spark: wave(48, 1.2, 0.5, 3) },
      'sensor.living_room_humidity': { name: 'Living room humidity', kind: 'sensor', value: '48%', numeric: true, unit: '%' },
      'binary_sensor.front_door': { name: 'Front door', kind: 'sensor', value: 'Off' },
      'sensor.bedroom_temperature': { name: 'Bedroom temperature', kind: 'sensor', value: '21.3 °C', numeric: true, unit: '°C' },
      'switch.water_pump': { name: 'Water pump', kind: 'toggle', on: false, detail: 'Off', value: 'Off' }
    }
  };
  function view(id) {
    var e = db.all[id];
    return {
      id: id, name: e.name, domain: id.split('.')[0], kind: e.kind, on: !!e.on, available: true,
      value: e.value, detail: e.detail || e.value, numeric: !!e.numeric,
      graph: db.graphs.indexOf(id) >= 0, widget: db.widgets.indexOf(id) >= 0,
      spark: db.graphs.indexOf(id) >= 0 ? e.spark : undefined
    };
  }
  function snapshot() {
    return {
      rev: db.rev, version: '0.1.0', configured: db.configured, connected: db.configured, error: '',
      url: db.configured ? 'http://homeassistant.local:8123' : '', name: 'Home', haVersion: '2026.11.1',
      theme: db.theme, themes: ['glass', 'midnight', 'neon', 'sunset'], total: Object.keys(db.all).length,
      widgets: db.widgets.length, maxWidgets: 5, entities: db.order.map(view),
      update: { available: params.has('update'), latest: '0.2.0', url: '' }
    };
  }
  function toggle(list, id, on) {
    var i = list.indexOf(id);
    if (on && i < 0) list.push(id);
    if (!on && i >= 0) list.splice(i, 1);
  }
  function handle(path, q, body) {
    db.rev++;
    switch (path) {
      case '/api/snapshot': db.rev--; return snapshot();
      case '/api/entities':
        db.rev--;
        return Object.keys(db.all).map(function (id) {
          var v = view(id);
          return { id: id, name: v.name, domain: v.domain, value: v.value, numeric: v.numeric, selected: db.order.indexOf(id) >= 0, graph: v.graph, widget: v.widget };
        });
      case '/api/history':
        db.rev--;
        var e = db.all[q.get('id')], vals = e.spark ? (e.unit === 'kW' && e.name[0] === 'S' ? bell(120, 4.1) : wave(120, parseFloat(e.value), 3, 1)) : wave(120, parseFloat(e.value), 2, 2);
        var hours = Number(q.get('hours')) || 24, now = Date.now();
        return { id: q.get('id'), name: e.name, unit: e.unit || '', value: e.value, hours: hours, from: now - hours * 3600e3, to: now, values: vals,
          stats: { min: Math.min.apply(null, vals), max: Math.max.apply(null, vals), avg: vals.reduce(function (a, b) { return a + b; }, 0) / vals.length } };
      case '/api/act':
        var t = db.all[body.id]; t.on = !t.on; t.detail = t.on ? 'On' : 'Off'; return { ok: true };
      case '/api/select':
        toggle(db.order, body.id, body.on);
        if (!body.on) { toggle(db.graphs, body.id, false); toggle(db.widgets, body.id, false); }
        return { ok: true };
      case '/api/graph': toggle(db.graphs, body.id, body.on); return { ok: true };
      case '/api/widget':
        if (body.on && db.widgets.length >= 5) throw new Error('the menu bar holds up to 5 widgets; remove one first');
        toggle(db.widgets, body.id, body.on); return { ok: true };
      case '/api/move':
        var i = db.order.indexOf(body.id), j = i + body.delta;
        if (i >= 0 && j >= 0 && j < db.order.length) { var x = db.order[i]; db.order[i] = db.order[j]; db.order[j] = x; }
        return { ok: true };
      case '/api/theme': db.theme = body.theme; return { ok: true };
      case '/api/connect':
        if (!body.url) throw new Error('enter the address of your Home Assistant');
        db.configured = true; return { ok: true };
      case '/api/disconnect': db.configured = false; return { ok: true };
    }
    return { ok: true };
  }
  window.fetch = function (url, opts) {
    var u = new URL(url, location.href);
    var body = opts && opts.body ? JSON.parse(opts.body) : undefined;
    try {
      var out = handle(u.pathname.replace(/^.*\/api\//, '/api/'), u.searchParams, body);
      return Promise.resolve({ ok: true, json: function () { return Promise.resolve(out); } });
    } catch (err) {
      return Promise.resolve({ ok: false, json: function () { return Promise.resolve({ error: err.message }); } });
    }
  };
})();
