'use strict';

/*
 * FleetFlow dashboard behaviour.
 *
 * Plain JavaScript served from the binary that answers the API: no build step,
 * no framework, and no request leaving this origin (DESIGN 3.6).
 *
 * Today it fetches the road network once from GET /api/graph and draws it on
 * both panels, so the two arms are compared over the same roads; wires the
 * controls to POST /api/sim/start and POST /api/sim/reset; and holds one
 * GET /api/stream connection open for everything that moves: vehicle positions,
 * edge congestion and the metrics strip. A run advances only while that
 * connection is open, so the page has to say so and has to keep it open.
 */

/* ---------- map constants ---------- */

// Roads are coloured by congestion: cost_s over free_flow_s, the ratio the stream
// carries. The ramp is the palette's own meaning — phosphor green for free flow,
// radar amber for a road carrying more than it was built for, and the error tone
// for one past saturation. The baseline panel is drawn at free flow by
// definition (DESIGN 3.5): its weights are fixed and never move, so it never
// sees the congestion the FleetFlow metric reports.
const ROAD_FREE = '#00ff88';
const ROAD_BUSY = '#ffaa00';
const ROAD_SEVERE = '#ffb4ab';
// ROAD_CONGESTED and ROAD_SATURATED are the ratios the colour changes at.
const ROAD_CONGESTED = 1.1;
const ROAD_SATURATED = 1.5;

const ROAD_MIN_WIDTH = 1;
const ROAD_LANE_WIDTH = 0.7;
const ROAD_MAX_WIDTH = 4;

// MAP_MARGIN keeps the network off the panel's own border, in CSS pixels.
const MAP_MARGIN = 12;

/* ---------- projection ---------- */

// boundsOf is the network's extent, in degrees, derived from the nodes it is
// given: the graph can be swapped for an OSM extract without this code or any
// coordinate changing.
function boundsOf(nodes) {
  const bounds = { minLat: Infinity, maxLat: -Infinity, minLon: Infinity, maxLon: -Infinity };
  for (const node of nodes) {
    bounds.minLat = Math.min(bounds.minLat, node.lat);
    bounds.maxLat = Math.max(bounds.maxLat, node.lat);
    bounds.minLon = Math.min(bounds.minLon, node.lon);
    bounds.maxLon = Math.max(bounds.maxLon, node.lon);
  }
  return bounds;
}

// project builds the mapping from a coordinate to a canvas pixel.
//
// One scale serves both axes, whichever fits, so the network keeps its true
// shape: stretching latitude to fill a wide panel would draw a road where there
// is none. Canvas y grows downward while latitude grows north, so the vertical
// axis is flipped, and the network is centred in the space the scale left.
//
// This is an equirectangular projection over a city-sized extract, where a
// degree of longitude and a degree of latitude are within a few percent of the
// same distance. It is not claimed to hold for a country-sized one.
function project(bounds, width, height, margin) {
  const spanLon = Math.max(bounds.maxLon - bounds.minLon, 1e-9);
  const spanLat = Math.max(bounds.maxLat - bounds.minLat, 1e-9);
  const usableW = Math.max(width - 2 * margin, 1);
  const usableH = Math.max(height - 2 * margin, 1);
  const scale = Math.min(usableW / spanLon, usableH / spanLat);
  const originX = (width - scale * spanLon) / 2;
  const originY = (height - scale * spanLat) / 2;

  return {
    scale,
    toX: (lon) => originX + (lon - bounds.minLon) * scale,
    toY: (lat) => originY + (bounds.maxLat - lat) * scale,
  };
}

/* ---------- panel state ---------- */

// newState is everything one panel draws from. The stream dispatch adds to it
// rather than reshaping it: the drawing below takes the state as a whole, so a
// panel that also has congestion and vehicles needs no new path.
function newState(graph) {
  const nodes = new Map();
  for (const node of graph.nodes) {
    nodes.set(node.id, node);
  }

  return {
    nodes,
    edges: graph.edges,
    bounds: boundsOf(graph.nodes),
    // Canvas size in CSS pixels, set by layout.
    width: 0,
    height: 0,
    dpr: 1,
    projection: null,
    // Edge id to {cost_s, free_flow_s}, from the stream's edges event (T51).
    congestion: null,
    // This panel's arm of the run, from the stream's vehicles event (T50).
    vehicles: [],
  };
}

function newPanel(arm, canvas) {
  return { arm, canvas, ctx: canvas ? canvas.getContext('2d') : null, state: null };
}

// panels is one entry per comparison panel. Both share one graph and one redraw
// path; only the data each holds differs.
const panels = [
  newPanel('baseline', document.getElementById('canvas-baseline')),
  newPanel('fleetflow', document.getElementById('canvas-fleetflow')),
];

// layout sizes the bitmap for the display's pixel ratio and rebuilds the
// projection. It is the only place the element's size is read, so a first paint
// and a resize take the same path.
function layout(panel) {
  const { canvas, state } = panel;
  if (!canvas || !state) return;

  const width = Math.max(1, Math.round(canvas.clientWidth));
  const height = Math.max(1, Math.round(canvas.clientHeight));
  const dpr = window.devicePixelRatio || 1;

  // The backing store is in device pixels and the drawing is in CSS pixels: a
  // 1px road stays 1px and crisp at any density, instead of being blurred across
  // two device pixels.
  canvas.width = Math.round(width * dpr);
  canvas.height = Math.round(height * dpr);

  state.width = width;
  state.height = height;
  state.dpr = dpr;
  state.projection = project(state.bounds, width, height, MAP_MARGIN);
}

// redrawPanel is a full paint of one panel from its state. Nothing is kept in
// the canvas between paints, so there is no stale geometry and no partial
// update to get wrong.
function redrawPanel(panel) {
  const { ctx, state } = panel;
  if (!ctx || !state || !state.projection) return;

  ctx.setTransform(state.dpr, 0, 0, state.dpr, 0, 0);
  ctx.clearRect(0, 0, state.width, state.height);
  drawRoads(ctx, state, panel.arm);
  drawVehicles(ctx, state);
}

// roadWidth is the free-flow width: a road with more lanes carries more and
// reads as wider.
function roadWidth(edge) {
  const lanes = Math.max(1, edge.lanes || 1);
  return Math.min(ROAD_MAX_WIDTH, ROAD_MIN_WIDTH + ROAD_LANE_WIDTH * (lanes - 1));
}

// roadColour is the congestion colour of one edge for one panel.
//
// The FleetFlow panel reads cost_s / free_flow_s from the stream's edge metric.
// The baseline arm's weights are fixed at free flow and never move, so its panel
// is drawn at free flow: colouring it with the FleetFlow metric would show
// congestion the baseline arm never sees, which is the one thing this comparison
// must not do (DESIGN 3.5, "One congestion frame, two panels").
function roadColour(state, arm, edge) {
  if (arm !== 'fleetflow' || !state.congestion) return ROAD_FREE;

  const measured = state.congestion.get(edge.id);
  if (!measured || !(measured.free_flow_s > 0)) return ROAD_FREE;

  const ratio = measured.cost_s / measured.free_flow_s;
  if (ratio >= ROAD_SATURATED) return ROAD_SEVERE;
  if (ratio >= ROAD_CONGESTED) return ROAD_BUSY;
  return ROAD_FREE;
}

// drawRoads draws every edge from the graph's own coordinates, at its free-flow
// width, coloured by congestion. Vehicle markers draw after this call, from the
// same state.
function drawRoads(ctx, state, arm) {
  const { toX, toY } = state.projection;

  // Round caps meet cleanly where several edges share a node.
  ctx.lineCap = 'round';
  ctx.lineJoin = 'round';

  for (const edge of state.edges) {
    const from = state.nodes.get(edge.from);
    const to = state.nodes.get(edge.to);
    // A missing endpoint is a fault in the graph, not a drawing decision.
    if (!from || !to) continue;

    ctx.strokeStyle = roadColour(state, arm, edge);
    ctx.lineWidth = roadWidth(edge);
    ctx.beginPath();
    ctx.moveTo(toX(from.lon), toY(from.lat));
    ctx.lineTo(toX(to.lon), toY(to.lat));
    ctx.stroke();
  }
}

/* ---------- vehicle markers (T50, T52, T55) ---------- */

// Marker colours. A fleet vehicle chooses its own route and is the subject of the
// comparison, so it takes the primary cyan; background traffic is drawn dim; a
// live vehicle is the one thing on the map that came from outside a run, so it
// takes the palette's rogue violet and is always labelled.
const VEHICLE_FLEET = '#00f3ff';
const VEHICLE_BACKGROUND = '#849495';
const VEHICLE_LIVE = '#a855f7';

// VEHICLE_LABEL_MAX is the count up to which every marker carries its word. A run
// can carry thousands of vehicles, and a label on each would be an unreadable
// smear; past this many only live markers — which are few — keep theirs, and the
// legend names the rest. The default run's 200 vehicles are split across the two
// panels, so a panel stays under this.
const VEHICLE_LABEL_MAX = 200;

// drawVehicles draws this panel's vehicles: the ones of its own arm, plus every
// live vehicle, which belongs to neither arm and so is drawn on both (T50). Each
// marker is labelled simulated or live (T55); the canvas carries the short form
// of each word, because a ten-character label per dot is not drawable.
function drawVehicles(ctx, state) {
  const { toX, toY } = state.projection;
  const vehicles = state.vehicles || [];
  const labelAll = vehicles.length <= VEHICLE_LABEL_MAX;

  ctx.font = "9px 'JetBrains Mono', 'Cascadia Mono', Consolas, monospace";
  ctx.textBaseline = 'middle';

  for (const vehicle of vehicles) {
    const x = toX(vehicle.lon);
    const y = toY(vehicle.lat);
    if (!isFinite(x) || !isFinite(y)) continue;

    const live = vehicle.kind === 'live';
    const colour = live ? VEHICLE_LIVE : vehicle.fleet ? VEHICLE_FLEET : VEHICLE_BACKGROUND;
    const radius = live ? 3 : vehicle.fleet ? 2.6 : 2;

    ctx.beginPath();
    ctx.arc(x, y, radius, 0, Math.PI * 2);
    if (live) {
      // A live vehicle is a ring around a dark core, so it reads as a sighting
      // rather than a solid fleet dot.
      ctx.fillStyle = 'rgba(7, 9, 14, 0.85)';
      ctx.fill();
      ctx.lineWidth = 1.2;
      ctx.strokeStyle = colour;
      ctx.stroke();
    } else {
      ctx.fillStyle = colour;
      ctx.fill();
    }

    if (live || labelAll) {
      ctx.fillStyle = colour;
      ctx.fillText(live ? 'live' : 'sim', x + radius + 3, y);
    }
  }
}

/* ---------- run controls ---------- */

// readAnswer turns one response into what the caller needs: the status, and the
// body when it carries one. Nothing is swallowed, because a rejected value has to
// be visible on the page and the API's error codes are what name the value that
// was rejected. A body that does not parse becomes null rather than a throw: a
// refusal from anything in front of the API is still an answer with a status.
async function readAnswer(res) {
  const text = await res.text();

  let parsed = null;
  if (text !== '') {
    try {
      parsed = JSON.parse(text);
    } catch (err) {
      parsed = null;
    }
  }
  return { status: res.status, body: parsed };
}

// getJSON reads one endpoint. Every fleet pane reads a list this way (T48), so a
// refused read is reported the same way wherever it happens.
async function getJSON(path) {
  const res = await fetch(path, { headers: { Accept: 'application/json' } });
  return readAnswer(res);
}

// postJSON sends one request and returns what the server answered.
async function postJSON(path, body) {
  const options = { method: 'POST', headers: { Accept: 'application/json' } };
  if (body !== undefined) {
    options.headers['Content-Type'] = 'application/json';
    options.body = JSON.stringify(body);
  }

  return readAnswer(await fetch(path, options));
}

// setNote writes one line of progress or refusal into a status line. Every
// request the page makes reports itself this way — the run controls into the one
// line below them, each fleet pane into its own (T48) — so a reader reads the
// same shape wherever they looked.
function setNote(id, text, kind) {
  const out = document.getElementById(id);
  if (!out) return;
  out.textContent = `>_ ${text}`;
  out.dataset.kind = kind;
}

function showResult(text, kind) {
  setNote('control-result', text, kind);
}

function setRunChip(text, state) {
  const chip = document.getElementById('run-chip');
  if (!chip) return;
  chip.textContent = text;
  chip.dataset.state = state;
}

// readNumber returns the field's value, null when it was left empty (the server
// then applies its own default), and NaN when it is not a number at all.
function readNumber(id) {
  const input = document.getElementById(id);
  if (!input || input.value.trim() === '') return null;
  return input.valueAsNumber;
}

function describeError(res) {
  if (res.body && res.body.code) {
    return `${res.status} ${res.body.code} — ${res.body.message}`;
  }
  return `${res.status} unexpected response`;
}

async function startRun() {
  const body = {
    seed: readNumber('seed'),
    vehicles: readNumber('vehicles'),
    fleet_share: readNumber('fleet-share'),
  };

  for (const [field, value] of Object.entries(body)) {
    if (Number.isNaN(value)) {
      showResult(`${field.replace('_', ' ')} is not a number`, 'error');
      return;
    }
  }

  showResult('POST /api/sim/start …', 'idle');
  try {
    const res = await postJSON('/api/sim/start', body);
    if (res.status === 202 && res.body) {
      setRunChip('Running', 'running');
      showResult(
        `202 accepted — seed ${res.body.seed}, ${res.body.vehicles} vehicles, ` +
          `fleet share ${res.body.fleet_share}, step ${res.body.step_s}s`,
        'ok'
      );
      return;
    }
    showResult(describeError(res), 'error');
  } catch (err) {
    showResult(`request failed: ${err.message}`, 'error');
  }
}

async function resetRun() {
  showResult('POST /api/sim/reset …', 'idle');
  try {
    const res = await postJSON('/api/sim/reset');
    if (res.status === 204) {
      setRunChip('No run', 'idle');
      showResult('204 reset — no run', 'ok');
      return;
    }
    showResult(describeError(res), 'error');
  } catch (err) {
    showResult(`request failed: ${err.message}`, 'error');
  }
}

function wireControls() {
  const start = document.getElementById('start');
  if (start) start.addEventListener('click', startRun);

  const reset = document.getElementById('reset');
  if (reset) reset.addEventListener('click', resetRun);
}

/* ---------- view switcher (T48) ---------- */

// The dashboard has two views. Comparison is what it has always shown, so it is
// the one the page opens on and nothing about it changes; Fleet is the
// dispatcher's records. Only one view is in the document's flow at a time, so a
// hidden canvas has no size to measure — which is why returning to the
// comparison view lays the panels out again rather than trusting what the last
// paint left (T49).
const VIEWS = ['comparison', 'fleet'];

function showView(name) {
  if (!VIEWS.includes(name)) return;

  for (const section of document.querySelectorAll('[data-view]')) {
    section.hidden = section.dataset.view !== name;
  }
  for (const view of VIEWS) {
    const button = document.getElementById('view-' + view);
    if (button) button.setAttribute('aria-pressed', view === name ? 'true' : 'false');
  }

  if (name === 'fleet') {
    refreshFleet();
    return;
  }

  // A canvas inside a display:none subtree reports no size, so returning to the
  // comparison view is a layout and a paint, exactly as a resize is (T49).
  for (const panel of panels) {
    layout(panel);
    redrawPanel(panel);
  }
}

function wireViews() {
  for (const view of VIEWS) {
    const button = document.getElementById('view-' + view);
    if (button) button.addEventListener('click', () => showView(view));
  }
}

/* ---------- fleet records (T48) ---------- */

// Every pane reads one list endpoint and writes through the endpoint that owns
// the change: a create, an assignment and a status move are each a request
// followed by a re-read, so the table shows what the server stored rather than
// what the page hoped it stored. Nothing here keeps a rule the API already
// applies -- forward-only trip statuses, one driver per vehicle -- because a
// second copy of a rule is a second thing to get wrong.
const fleetState = {
  vehicles: [],
  drivers: [],
  trips: [],
  runs: [],
  // errors holds the last failure per pane. A pane that could not be read says
  // why: an empty table must not read as "no records" when the answer was 503
  // fleet_unavailable (T48).
  errors: {},
};

// el builds one element with an optional class and text, so a render function
// reads as the row it draws rather than as a wall of createElement calls.
function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined && text !== null) node.textContent = text;
  return node;
}

// textOf renders a value the API left out as an em dash rather than as a blank
// cell, so "unassigned" is visible and not an empty slot.
function textOf(value) {
  return value === undefined || value === null || value === '' ? '\u2014' : String(value);
}

// emptyRow fills a table body with the one line that says why it has no records:
// nothing yet, or the failure the endpoint answered with.
function emptyRow(tbody, columns, text) {
  tbody.replaceChildren();
  const cell = el('td', 'empty', text);
  cell.colSpan = columns;
  const row = el('tr');
  row.appendChild(cell);
  tbody.appendChild(row);
}

// readList reads one list endpoint. A list is only a list when the body carries
// the array the contract names; anything else is thrown, so the pane reports the
// server's own message instead of drawing an empty table.
async function readList(path, key) {
  const res = await getJSON(path);
  if (res.status !== 200 || !res.body || !Array.isArray(res.body[key])) {
    throw new Error(describeError(res));
  }
  return res.body[key];
}

function setPaneNote(pane, text, kind) {
  setNote(pane + '-note', text, kind);
}

// noteCount is the one line a pane shows once it has read its endpoint: how many
// records it got, or that there are none yet.
function noteCount(pane, count, singular) {
  if (count === 0) {
    setPaneNote(pane, `nothing yet \u2014 no ${singular}s on record`, 'idle');
    return;
  }
  setPaneNote(pane, `${count} ${singular}${count === 1 ? '' : 's'}`, 'ok');
}

// failPane reports a failed read in the pane that asked for it, in the API's own
// words, and leaves that pane's table saying the same thing.
function failPane(pane, err) {
  fleetState.errors[pane] = err.message;
  setPaneNote(pane, err.message, 'error');
}

// refreshFleet reads every pane. It is what the Fleet view does when it is
// opened, so the tables are the server's answer at the moment they are looked
// at rather than whatever the last visit left behind.
function refreshFleet() {
  return Promise.all([refreshVehicles(), refreshDrivers(), refreshTrips(), refreshHistory()]);
}

async function refreshVehicles() {
  setPaneNote('vehicles', 'GET /api/vehicles \u2026', 'idle');
  try {
    // The drivers are read with the vehicles because each row shows the driver
    // holding it: the assignment is visible from both tables, so neither is a
    // list of ids a reader has to join by hand.
    const [vehicles, drivers] = await Promise.all([
      readList('/api/vehicles', 'vehicles'),
      readList('/api/drivers', 'drivers'),
    ]);
    delete fleetState.errors.vehicles;
    fleetState.vehicles = vehicles;
    fleetState.drivers = drivers;
    renderVehicles();
    noteCount('vehicles', vehicles.length, 'vehicle');
  } catch (err) {
    failPane('vehicles', err);
    renderVehicles();
  }
}

// driverOnVehicle names the driver holding a vehicle, or null. The API answers
// the assignment from the vehicle too, but a reloaded table reads every driver
// at once, so the join is made here rather than with a request per row.
function driverOnVehicle(vehicleID) {
  return fleetState.drivers.find((driver) => driver.vehicle_id === vehicleID) || null;
}

async function refreshDrivers() {
  setPaneNote('drivers', 'GET /api/drivers \u2026', 'idle');
  try {
    const drivers = await readList('/api/drivers', 'drivers');
    delete fleetState.errors.drivers;
    fleetState.drivers = drivers;
    renderDrivers();
    noteCount('drivers', drivers.length, 'driver');
  } catch (err) {
    failPane('drivers', err);
    renderDrivers();
  }
}

function renderDrivers() {
  const tbody = document.getElementById('drivers-rows');
  if (!tbody) return;

  if (fleetState.errors.drivers) {
    emptyRow(tbody, 4, fleetState.errors.drivers);
    return;
  }
  if (fleetState.drivers.length === 0) {
    emptyRow(tbody, 4, 'No drivers yet.');
    return;
  }

  tbody.replaceChildren();
  for (const driver of fleetState.drivers) {
    const row = el('tr');
    row.appendChild(el('td', 'mono', driver.id));
    row.appendChild(el('td', null, driver.name));
    row.appendChild(el('td', null, textOf(driver.contact)));
    // The vehicle the driver holds travels back from the API, so an assignment
    // is visible from the driver's side of it and not only the vehicle's.
    row.appendChild(el('td', 'mono', textOf(driver.vehicle_id)));
    tbody.appendChild(row);
  }
}

// nextTripStatus is the one status a trip can move to from where it is:
// planned, then in progress, then completed. A finished trip is a record of what
// happened, so it offers no move.
const nextTripStatus = { planned: 'in_progress', in_progress: 'completed' };

async function refreshTrips() {
  setPaneNote('trips', 'GET /api/trips \u2026', 'idle');
  try {
    const trips = await readList('/api/trips', 'trips');
    delete fleetState.errors.trips;
    fleetState.trips = trips;
    renderTrips();
    noteCount('trips', trips.length, 'trip');
  } catch (err) {
    failPane('trips', err);
    renderTrips();
  }
}

function renderTrips() {
  const tbody = document.getElementById('trips-rows');
  if (!tbody) return;

  if (fleetState.errors.trips) {
    emptyRow(tbody, 6, fleetState.errors.trips);
    return;
  }
  if (fleetState.trips.length === 0) {
    emptyRow(tbody, 6, 'No trips yet.');
    return;
  }

  tbody.replaceChildren();
  for (const trip of fleetState.trips) {
    const row = el('tr');
    row.appendChild(el('td', 'mono', trip.id));
    row.appendChild(el('td', null, trip.origin));
    row.appendChild(el('td', null, trip.destination));
    row.appendChild(el('td', 'mono', textOf(trip.vehicle_id)));
    row.appendChild(el('td', 'mono', trip.status));
    tbody.appendChild(row);
  }
}

// completedTrips keeps the trips that finished, in the order the API answered
// them: history is a record of what happened, not a second sort of the same list.
function completedTrips(trips) {
  return trips.filter((trip) => trip.status === 'completed');
}

// savedRunMetric reads one figure out of a saved run's own metrics document. The
// document is the object GET /api/metrics reports under "run", so the two
// comparison arms are resolved by the same rule the metrics strip uses. A figure
// the run never produced is an em dash, not a zero: zero is a measurement.
function savedRunMetric(run, path) {
  return formatMetric(path, metricAt({ run: run.metrics }, path));
}

// savedAt renders a run's created_at in the reader's own clock: the API sends
// UTC, and the person reading saved it in local time.
function savedAt(value) {
  const at = new Date(value);
  if (Number.isNaN(at.getTime())) return textOf(value);
  return at.toLocaleString();
}

async function refreshHistory() {
  setPaneNote('history', 'GET /api/trips and GET /api/runs \u2026', 'idle');
  try {
    // History is two records in one pane: the trips that finished, and the runs
    // that were saved. Both are read together because one pane shows both, and a
    // trip that finishes is history from that moment.
    const [trips, runs] = await Promise.all([
      readList('/api/trips', 'trips'),
      readList('/api/runs', 'runs'),
    ]);
    delete fleetState.errors.history;
    fleetState.trips = trips;
    fleetState.runs = runs;
    renderHistory();
    renderTrips();
    const completed = completedTrips(trips).length;
    setPaneNote(
      'history',
      `${completed} completed trip${completed === 1 ? '' : 's'}, ` +
        `${runs.length} saved run${runs.length === 1 ? '' : 's'}`,
      'ok'
    );
  } catch (err) {
    failPane('history', err);
    renderHistory();
  }
}

function renderHistory() {
  const tripsBody = document.getElementById('history-trips-rows');
  const runsBody = document.getElementById('history-runs-rows');
  if (!tripsBody || !runsBody) return;

  if (fleetState.errors.history) {
    emptyRow(tripsBody, 4, fleetState.errors.history);
    emptyRow(runsBody, 4, fleetState.errors.history);
    return;
  }

  const completed = completedTrips(fleetState.trips);
  if (completed.length === 0) {
    emptyRow(tripsBody, 4, 'No completed trips yet.');
  } else {
    tripsBody.replaceChildren();
    for (const trip of completed) {
      const row = el('tr');
      row.appendChild(el('td', 'mono', trip.id));
      row.appendChild(el('td', null, trip.origin));
      row.appendChild(el('td', null, trip.destination));
      row.appendChild(el('td', 'mono', textOf(trip.vehicle_id)));
      tripsBody.appendChild(row);
    }
  }

  if (fleetState.runs.length === 0) {
    emptyRow(runsBody, 4, 'No saved runs yet \u2014 save one from the comparison controls.');
    return;
  }

  runsBody.replaceChildren();
  for (const run of fleetState.runs) {
    const baseline = savedRunMetric(run, 'baseline.average_trip_s');
    const fleetflow = savedRunMetric(run, 'fleet_flow.average_trip_s');
    const row = el('tr');
    row.appendChild(el('td', 'mono', run.id));
    row.appendChild(el('td', 'mono', run.seed));
    row.appendChild(el('td', 'mono', savedAt(run.created_at)));
    row.appendChild(el('td', 'mono', `B ${baseline} / F ${fleetflow}`));
    runsBody.appendChild(row);
  }
}

function renderVehicles() {
  const tbody = document.getElementById('vehicles-rows');
  if (!tbody) return;

  if (fleetState.errors.vehicles) {
    emptyRow(tbody, 4, fleetState.errors.vehicles);
    return;
  }
  if (fleetState.vehicles.length === 0) {
    emptyRow(tbody, 4, 'No vehicles yet.');
    return;
  }

  tbody.replaceChildren();
  for (const vehicle of fleetState.vehicles) {
    const row = el('tr');
    row.appendChild(el('td', 'mono', vehicle.id));
    row.appendChild(el('td', null, vehicle.name));
    row.appendChild(el('td', null, textOf(vehicle.type)));
    row.appendChild(el('td', 'mono', vehicle.status));
    tbody.appendChild(row);
  }
}

/* ---------- fleet changes (T48) ---------- */

// Every change below is one request and then a re-read of the panes that change
// can move. The re-read is what keeps the tables honest: a trip that finishes
// frees its vehicle, and only the server knows that.
//
// A failure is reported in the pane it came from, in the API's own words, and
// never as a silent no-op: a create refused for a blank name, or a move refused
// because it goes backwards, is something the dispatcher has to see.

// readField returns a form field's value, trimmed. An absent field is the empty
// string, which the API refuses in its own words rather than this page guessing.
function readField(id) {
  const input = document.getElementById(id);
  return input ? input.value.trim() : '';
}

async function createVehicle(event) {
  event.preventDefault();
  const body = {
    name: readField('vehicle-name'),
    type: readField('vehicle-type'),
    status: readField('vehicle-status'),
  };

  setPaneNote('vehicles', 'POST /api/vehicles \u2026', 'idle');
  try {
    const res = await postJSON('/api/vehicles', body);
    if (res.status !== 201) {
      setPaneNote('vehicles', describeError(res), 'error');
      return;
    }
    document.getElementById('vehicle-form').reset();
    setPaneNote('vehicles', `201 created ${res.body.name} (${res.body.id})`, 'ok');
    // The trip form's vehicle select and the trips table both name vehicles, so
    // a new one changes more than the table it was created in.
    await refreshVehicles();
    await refreshTrips();
  } catch (err) {
    setPaneNote('vehicles', `request failed: ${err.message}`, 'error');
  }
}

function wireFleetForms() {
  const form = document.getElementById('vehicle-form');
  if (form) form.addEventListener('submit', createVehicle);

  for (const button of document.querySelectorAll('[data-refresh]')) {
    button.addEventListener('click', () => refreshPane(button.dataset.refresh));
  }
}

// refreshPane re-reads one pane by name, which is what its Refresh button asks
// for. One button per pane rather than one for the view: a dispatcher watching
// trips does not need the history re-read to see the next status.
function refreshPane(name) {
  switch (name) {
    case 'vehicles':
      return refreshVehicles();
    case 'drivers':
      return refreshDrivers();
    case 'trips':
      return refreshTrips();
    case 'history':
      return refreshHistory();
    default:
      return undefined;
  }
}

/* ---------- startup ---------- */

async function fetchGraph() {
  const res = await fetch('/api/graph', { headers: { Accept: 'application/json' } });
  if (!res.ok) {
    throw new Error(`GET /api/graph answered ${res.status}`);
  }
  return res.json();
}

function setRoadCount(text) {
  const meta = document.getElementById('graph-meta');
  if (meta) meta.textContent = text;
}

async function init() {
  // The controls are wired before the graph is fetched, so a dashboard that
  // cannot draw the network can still start and reset a run.
  wireControls();
  wireViews();
  wireFleetForms();

  // The stream is opened before the graph is fetched, and independently of it:
  // the metrics strip and the run chip do not need the road network, and the
  // connection is what lets a run advance at all (T44).
  openStream();

  let graph;
  try {
    graph = await fetchGraph();
  } catch (err) {
    // Said on the page rather than only in the console: a dashboard with blank
    // panels should tell you why, and this is the one figure that proves the
    // network arrived.
    setRoadCount(`Roads unavailable: ${err.message}`);
    return;
  }
  setRoadCount(`Roads ${graph.edges.length} edges / ${graph.nodes.length} nodes`);

  for (const panel of panels) {
    panel.state = newState(graph);
    layout(panel);
    redrawPanel(panel);
  }

  repaintOnResize();
}

// repaintOnResize rebuilds the projection and repaints when a panel changes
// size. ResizeObserver watches the element, which covers a panel resizing for
// any reason — a window drag, a narrow layout, a phone turning — rather than
// only a window resize.
function repaintOnResize() {
  const repaint = () => {
    for (const panel of panels) {
      layout(panel);
      redrawPanel(panel);
    }
  };

  if (typeof ResizeObserver === 'function') {
    const observer = new ResizeObserver(repaint);
    for (const panel of panels) {
      observer.observe(panel.canvas);
    }
    return;
  }
  window.addEventListener('resize', repaint);
}

/* ---------- metrics strip (T53) ---------- */

// Every [data-metric] in index.html names a path into GET /api/metrics. The two
// comparison arms live under the run, so a path that starts with an arm is
// resolved there; everything else is resolved from the body's root. The Go test
// in internal/api replays this rule against a real metric body, so a renamed
// metric breaks the build rather than blanking a cell.
function metricAt(body, path) {
  const parts = path.split('.');
  let node = body;
  if (parts[0] === 'baseline' || parts[0] === 'fleet_flow') {
    node = body ? body.run : null;
  }
  for (const part of parts) {
    if (node === null || typeof node !== 'object') return undefined;
    node = node[part];
  }
  return node;
}

// metricUnit names a figure's unit from its path, so a new cell needs no new
// table: the pressures end in _s, the percentage in _pct, the engine's latencies
// in _ms.
function metricUnit(path) {
  if (path.includes('query_latency_ms')) return ' ms';
  if (path.endsWith('_pct')) return '%';
  if (path.endsWith('_s')) return ' s';
  return '';
}

// formatMetric renders a figure for a person: one decimal and a unit, and an em
// dash when the run has not produced it.
function formatMetric(path, value) {
  if (typeof value !== 'number' || !isFinite(value)) return '\u2014';

  // A figure with no unit is a count -- route changes, today -- and a count read
  // as "0.0" invites the question of what the fraction was. Anything with a unit
  // keeps its decimal, because the differences being compared are small ones.
  const unit = metricUnit(path);
  return value.toFixed(unit === '' ? 0 : 1) + unit;
}

// applyMetrics fills the strip from one metrics frame. The frame is exactly the
// body of GET /api/metrics, whether it was asked for once or arrived on the
// stream, so there is one shape and one place that reads it.
function applyMetrics(body) {
  for (const cell of document.querySelectorAll('[data-metric]')) {
    cell.textContent = formatMetric(cell.dataset.metric, metricAt(body, cell.dataset.metric));
  }

  const running = !!(body && body.run);
  const runState = document.getElementById('run-state');
  if (runState) {
    runState.textContent = running ? `run \u00b7 ${body.run.vehicles} vehicles` : 'no run';
  }
}

/* ---------- event stream (T55) ---------- */

// One Server-Sent Events connection carries the three events, each at its own
// cadence. EventSource reconnects on its own, but the wait it picks between
// attempts is the browser's to choose and can stretch into many seconds, so a
// dropped connection is handled here instead: the source is closed and reopened
// on a short, explicit backoff. A run only advances while somebody is connected
// (T44), so a stream left down is a run left standing still.
const STREAM_URL = '/api/stream';
const STREAM_RETRY_START_MS = 500;
const STREAM_RETRY_MAX_MS = 8000;

let stream = null;
let retryDelay = STREAM_RETRY_START_MS;
let retryTimer = null;

function setStreamChip(text, state) {
  const chip = document.getElementById('stream-chip');
  if (!chip) return;
  chip.textContent = text;
  chip.dataset.state = state;
}

// onStreamMessage parses one event's data and hands it to the handler. A payload
// that does not parse is dropped rather than thrown: one bad frame must not take
// the connection down.
function onStreamMessage(event, handler) {
  let payload;
  try {
    payload = JSON.parse(event.data);
  } catch (err) {
    return;
  }
  handler(payload);
}

function openStream() {
  if (stream) return;

  stream = new EventSource(STREAM_URL);

  stream.addEventListener('open', () => {
    retryDelay = STREAM_RETRY_START_MS;
    setStreamChip('Stream live', 'running');
  });
  stream.addEventListener('metrics', (event) => onStreamMessage(event, applyMetrics));
  stream.addEventListener('edges', (event) => onStreamMessage(event, applyEdges));
  stream.addEventListener('vehicles', (event) => onStreamMessage(event, applyVehicles));
  stream.addEventListener('error', reconnectStream);
}

// applyEdges stores one congestion frame, keyed by edge id, on the panel that
// reads it. The frame carries every edge, so one event paints the whole map.
// Both numbers are kept rather than reduced to a ratio: the pair is what makes
// the colour explicable. The baseline panel is not given the map at all — its
// weights are fixed at free flow, so it has no congestion to read (T51).
function applyEdges(frame) {
  const congestion = new Map();
  if (frame && Array.isArray(frame.edges)) {
    for (const edge of frame.edges) {
      congestion.set(edge.edge_id, edge);
    }
  }

  for (const panel of panels) {
    if (!panel.state) continue;
    if (panel.arm === 'fleetflow') panel.state.congestion = congestion;
    redrawPanel(panel);
  }
}

// applyVehicles distributes one positions frame across the panels (T50, T52).
// A simulated vehicle goes to the panel of its arm and nowhere else, so each
// panel shows its own arm over the same network. A live vehicle carries no arm
// because it belongs to neither, and is drawn on both.
function applyVehicles(frame) {
  const vehicles = frame && Array.isArray(frame.vehicles) ? frame.vehicles : [];

  for (const panel of panels) {
    if (!panel.state) continue;
    panel.state.vehicles = vehicles.filter(
      (vehicle) => vehicle.arm === panel.arm || vehicle.arm === undefined || vehicle.arm === null
    );
    redrawPanel(panel);
  }

  // The frame is the authority on whether a run is going, so a run started or
  // reset in another tab is reflected here too.
  setRunChip(frame && frame.running ? 'Running' : 'No run', frame && frame.running ? 'running' : 'idle');
}

// reconnectStream closes the failed source and schedules a reopen on a short
// backoff that doubles to a ceiling, so a server restart is picked up in half a
// second but a down server is not hammered.
function reconnectStream() {
  if (stream) {
    stream.close();
    stream = null;
  }
  if (retryTimer !== null) return;

  setStreamChip('Reconnecting\u2026', 'retry');
  retryTimer = window.setTimeout(() => {
    retryTimer = null;
    openStream();
  }, retryDelay);
  retryDelay = Math.min(retryDelay * 2, STREAM_RETRY_MAX_MS);
}

/* ---------- startup ---------- */

// init waits for the whole file to have been evaluated before it runs: the
// stream it opens reads bindings declared below, and a call before those exist
// would see them in their temporal dead zone. A classic script at the end of the
// body still runs while the document is loading, so the listener is the path a
// browser takes; the immediate call is for a page that is already parsed.
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', init);
} else {
  init();
}
