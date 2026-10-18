'use strict';

/*
 * FleetFlow dashboard behaviour.
 *
 * Plain JavaScript served from the binary that answers the API: no build step,
 * no framework, and no request leaving this origin (DESIGN 3.6).
 *
 * Today it fetches the road network once from GET /api/graph and draws it on
 * both panels, so the two arms are compared over the same roads. It does not
 * open GET /api/stream yet on purpose: a stream wired halfway is harder to
 * reason about than none. The seam it plugs into is at the end of this file.
 */

/* ---------- map constants ---------- */

// Roads carry no congestion yet, so they are drawn in the palette's structural
// outline. When the stream arrives, this becomes a function of cost_s over
// free_flow_s rather than a constant.
const ROAD_COLOR = '#3a494b';
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
  drawRoads(ctx, state);
}

// roadWidth is the free-flow width: a road with more lanes carries more and
// reads as wider.
function roadWidth(edge) {
  const lanes = Math.max(1, edge.lanes || 1);
  return Math.min(ROAD_MAX_WIDTH, ROAD_MIN_WIDTH + ROAD_LANE_WIDTH * (lanes - 1));
}

// drawRoads draws every edge from the graph's own coordinates, at its free-flow
// width and colour. Congestion colouring and vehicle markers draw after this
// call, from the same state.
function drawRoads(ctx, state) {
  const { toX, toY } = state.projection;

  ctx.strokeStyle = ROAD_COLOR;
  // Round caps meet cleanly where several edges share a node.
  ctx.lineCap = 'round';
  ctx.lineJoin = 'round';

  for (const edge of state.edges) {
    const from = state.nodes.get(edge.from);
    const to = state.nodes.get(edge.to);
    // A missing endpoint is a fault in the graph, not a drawing decision.
    if (!from || !to) continue;

    ctx.lineWidth = roadWidth(edge);
    ctx.beginPath();
    ctx.moveTo(toX(from.lon), toY(from.lat));
    ctx.lineTo(toX(to.lon), toY(to.lat));
    ctx.stroke();
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

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', init);
} else {
  init();
}

/*
 * ---- The stream seam (next dispatch: T50, T51, T53, T55) ----
 *
 * GET /api/stream is not opened yet, deliberately. The three events it carries
 * land in three different places, and each one repaints from the state above:
 *
 *   vehicles  {now_s, running, vehicles:[{id, kind, arm, edge_id, lat, lon, ...}]}
 *             five times a second. Keep the entries whose `arm` is this panel's,
 *             write them to state.vehicles and redrawPanel(). A live vehicle has
 *             no arm (it belongs to neither panel) and is drawn on both; `kind`
 *             is "simulated" or "live", and each marker is labelled with it.
 *   edges     {metric_version, edges:[{edge_id, cost_s, free_flow_s}]} once a
 *             second. Write them to state.congestion keyed by edge id and
 *             redrawPanel(). Congestion reads as cost_s / free_flow_s, so both
 *             numbers are kept: dividing here would lose the pair that makes the
 *             colour explicable.
 *   metrics   exactly the body of GET /api/metrics, once a second: fill the
 *             [data-metric] cells in the strip from run.baseline, run.fleet_flow
 *             and engine.
 *
 * A run only advances while somebody is watching the stream, so opening the
 * connection is also what makes Start produce movement; a dropped connection
 * has to be retried rather than reported once.
 */
