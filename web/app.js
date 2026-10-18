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

// postJSON sends one request and returns what the server answered: status and
// body both. Nothing is swallowed, because a rejected value has to be visible on
// the page and the API's error codes are what name the value that was rejected.
async function postJSON(path, body) {
  const options = { method: 'POST', headers: { Accept: 'application/json' } };
  if (body !== undefined) {
    options.headers['Content-Type'] = 'application/json';
    options.body = JSON.stringify(body);
  }

  const res = await fetch(path, options);
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

function showResult(text, kind) {
  const out = document.getElementById('control-result');
  if (!out) return;
  out.textContent = `>_ ${text}`;
  out.dataset.kind = kind;
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
