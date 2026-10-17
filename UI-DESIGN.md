---
name: Abyssal Speculative Archive
colors:
  surface: '#111319'
  surface-dim: '#111319'
  surface-bright: '#37393f'
  surface-container-lowest: '#0c0e13'
  surface-container-low: '#191c21'
  surface-container: '#1d2025'
  surface-container-high: '#282a30'
  surface-container-highest: '#33353b'
  on-surface: '#e2e2ea'
  on-surface-variant: '#b9cacb'
  inverse-surface: '#e2e2ea'
  inverse-on-surface: '#2e3036'
  outline: '#849495'
  outline-variant: '#3a494b'
  surface-tint: '#00dce6'
  primary: '#e3fdff'
  on-primary: '#00373a'
  primary-container: '#00f3ff'
  on-primary-container: '#006b71'
  inverse-primary: '#00696f'
  secondary: '#f5fff3'
  on-secondary: '#003919'
  secondary-container: '#34ff8d'
  on-secondary-container: '#007239'
  tertiary: '#fff7f1'
  on-tertiary: '#452b00'
  tertiary-container: '#ffd59f'
  on-tertiary-container: '#855700'
  error: '#ffb4ab'
  on-error: '#690005'
  error-container: '#93000a'
  on-error-container: '#ffdad6'
  primary-fixed: '#6ff6ff'
  primary-fixed-dim: '#00dce6'
  on-primary-fixed: '#002022'
  on-primary-fixed-variant: '#004f53'
  secondary-fixed: '#60ff99'
  secondary-fixed-dim: '#00e479'
  on-secondary-fixed: '#00210c'
  on-secondary-fixed-variant: '#005228'
  tertiary-fixed: '#ffddb4'
  tertiary-fixed-dim: '#ffb952'
  on-tertiary-fixed: '#291800'
  on-tertiary-fixed-variant: '#633f00'
  background: '#111319'
  on-background: '#e2e2ea'
  surface-variant: '#33353b'
typography:
  display-hero:
    fontFamily: Space Grotesk
    fontSize: 56px
    fontWeight: '700'
    lineHeight: 64px
    letterSpacing: -0.03em
  display-hero-mobile:
    fontFamily: Space Grotesk
    fontSize: 36px
    fontWeight: '700'
    lineHeight: 44px
    letterSpacing: -0.02em
  headline-lg:
    fontFamily: Space Grotesk
    fontSize: 32px
    fontWeight: '600'
    lineHeight: 40px
    letterSpacing: -0.02em
  headline-lg-mobile:
    fontFamily: Space Grotesk
    fontSize: 24px
    fontWeight: '600'
    lineHeight: 32px
    letterSpacing: -0.01em
  headline-md:
    fontFamily: Space Grotesk
    fontSize: 20px
    fontWeight: '600'
    lineHeight: 28px
    letterSpacing: -0.01em
  body-lg:
    fontFamily: Geist
    fontSize: 16px
    fontWeight: '400'
    lineHeight: 24px
  body-md:
    fontFamily: Geist
    fontSize: 14px
    fontWeight: '400'
    lineHeight: 20px
  body-sm:
    fontFamily: Geist
    fontSize: 12px
    fontWeight: '400'
    lineHeight: 18px
  code-telemetry:
    fontFamily: JetBrains Mono
    fontSize: 13px
    fontWeight: '500'
    lineHeight: 18px
    letterSpacing: 0.04em
  label-data:
    fontFamily: JetBrains Mono
    fontSize: 11px
    fontWeight: '600'
    lineHeight: 14px
    letterSpacing: 0.08em
  label-micro:
    fontFamily: JetBrains Mono
    fontSize: 9px
    fontWeight: '700'
    lineHeight: 12px
    letterSpacing: 0.12em
spacing:
  gutter: 1rem
  gutter-desktop: 1.5rem
  margin: 1rem
  margin-desktop: 2.5rem
  space-xs: 0.25rem
  space-sm: 0.5rem
  space-md: 1rem
  space-lg: 1.5rem
  space-xl: 2.5rem
---

## Brand & Style

The design system embodies the tension between deep nautical abyss research and forbidden rogue signal decryption. It creates an atmosphere of cold isolation, classified scientific discovery, and eerie technological immersion. The aesthetic merges tactical telemetry, benthic research terminals, and glitch-laden signal intelligence. 

The visual style blends **Brutalism** and **Tactile Telemetry**:
- **Tactical Telemetry & Monospace Architecture:** Dense instrumentation, grid-aligned coordinate systems, depth indicators, and frequency modulators evoke real-world oceanic probe interfaces.
- **Bioluminescent High-Contrast:** Pitch-black backgrounds are pierced by sharp, hyper-saturated luminous data streams, mimicking organism bioluminescence and cathode-ray tube phosphor displays.
- **Controlled Glitch & Degradation:** Subtle chromatic aberrations, scanlines, crosshair reticles, and border tick marks reinforce the feeling of compromised, low-bandwidth data retrieved from extreme depths.

## Colors

The palette simulates an abyssal descent into sensory deprivation, punctuated by radiant sensory cues and alert signatures:

- **Primary (`#00f3ff` - Bioluminescent Cyan):** The core telemetry signature. Used for primary interactive states, dominant telemetry metrics, radar rings, and active data streams.
- **Secondary (`#00ff88` - Phosphor Green):** Represents biological discovery, sonar ping returns, signal stability, and nominal subsystem health.
- **Tertiary (`#ffaa00` - Radar Amber):** Denotes telemetry anomalies, localized depth pressure alerts, signal degradation, and archival warnings.
- **Signal Glitch (`#a855f7` - Rogue Violet):** Reserved for untrusted data packets, corrupted deep-sea audio transcripts, and anomalous entities.
- **Background Abyssal Tiers:**
  - Base Void: `#07090e` (Primary canvas canvas; total blackness).
  - Sub-Trench Surface: `#0c1017` (Panels, telemetry modules, inspector sidebars).
  - Elevated Node Surface: `#131b26` (Hovered entries, active radar blips, modal cards).
  - Structural Reticle / Border: `#1c2838` (Precision framing, tick marks, dividing rules).

## Typography

Typography prioritizes extreme legible density, structural alignment, and technical precision:

- **Display & Headlines (`Space Grotesk`):** Applied to classified sector designations, anomaly codenames, and primary dashboard metrics. Crisp geometric proportions reflect advanced deep-sea instrumentation.
- **Body (`Geist`):** Delivers clean, unhindered reading for speculative logs, decoded mission transcripts, and system telemetry descriptions without causing eye fatigue in dark modes.
- **Labels, Readouts & Telemetry (`JetBrains Mono`):** Applied to depth gauges, coordinates, frequencies, timestamps, and hex dumps. Rendered strictly in uppercase for labels (`label-data`, `label-micro`) with extended letter-spacing to reinforce tactical HUD readability.

## Layout & Spacing

The layout operates on a 12-column rigid tactical grid with explicit coordinate markers, corner crosshairs, and modular pane dividers.

- **Grid Discipline:** Content frames conform to viewport edges like a submarine command console. The desktop experience uses fixed modular panels (left nav/telemetry, center radar/simulation visualizer, right dossier/metadata inspector) separated by `1px` structural borders (`#1c2838`).
- **Responsive Adaptation:**
  - **Desktop (1280px+):** Tri-panel mission terminal with persistent depth gauges, audio frequency waterfall visualizer, and anomalous node canvas.
  - **Tablet (768px - 1279px):** Split layout; collapsible dossier inspector docked to the bottom or slide-out drawer. Gutters scale to `1rem`.
  - **Mobile (<768px):** Single-column stacked telemetry stack with pinned bottom frequency tuner/navigation switchers. Outer margin tightens to `1rem`.
- **Rhythm:** Spacing between atomic data elements (coordinates, sensor values) stays compressed (`space-xs` to `space-sm`), while major archival entries maintain strict module separations (`space-lg` to `space-xl`).

## Elevation & Depth

Visual depth is achieved through layered benthic darkness, illuminated scanlines, and targeted luminescent glows rather than generic drop shadows:

- **Tonal Tiers:** Level 0 is the baseline abyss (`#07090e`). Surface modules sit at Level 1 (`#0c1017`). Hovered nodes, active scanners, and popover dossiers rest at Level 2 (`#131b26`).
- **Bioluminescent Glow:** Interactive components and critical alarms emit localized radial glows using high-intensity color overlays with tight radii (e.g., `box-shadow: 0 0 16px rgba(0, 243, 255, 0.25)` for primary interactions; `0 0 20px rgba(168, 85, 247, 0.3)` for rogue signals).
- **Sub-Aquatic Scanlines & Textures:** Elevated cards feature a faint CSS linear-gradient scanline overlay (1px lines with 4px repeating pitch, 3% opacity) to evoke old phosphor-coated radar scopes.
- **Ghost Outlines:** Containers avoid opaque shadows, using crisp, low-opacity borders (`1px solid #1c2838` transitioning to `1px solid #00f3ff` on active state).

## Shapes

The interface embraces a strict, zero-radius (`0px`) industrial aesthetic. 

- **Corner Geometry:** All buttons, data modules, cards, and modal windows feature square edges. Key telemetry cards utilize 45-degree chamfered cut corners (`clip-path: polygon(...)`) with 6px to 8px bevels to mimic reinforced hull plating and technical casing.
- **Reticle Indicators:** Framing elements include micro corner brackets (`+` or `L` bracket markers at card corners) rendered via pseudo-elements to reinforce targeting accuracy.

## Components

### Buttons & Telemetry Triggers
- **Primary Action (Scan / Ping / Decrypt):** Sharp rectangular shape with a `1px solid #00f3ff` border, transparent background, and `JetBrains Mono` uppercase text in `#00f3ff`. Hover triggers a full background flood of `#00f3ff` with `#07090e` inverted text and a cyan edge flare.
- **Secondary Action (Isolate / Calibrate):** Border in `#1c2838`, text in phosphor green (`#00ff88`). Hover shifts border to `#00ff88` with an ambient glow.
- **Danger / Anomaly Purge:** Border in `#a855f7`, violet glow pulse, text in `#a855f7`.

### Interactive Telemetry Nodes & Map Pings
- Represent anomalies in the abyss visualizer. Consists of a central pulsating dot (4px) enveloped by dual concentric radar rings that expand and fade over 2.4s intervals. Hovering reveals a contextual micro-HUD listing depth in meters (`-8,421m`), signal integrity (`44.2%`), and classified designation code.

### Dossier & Archive Cards
- Encased in `#0c1017` with a 1px border (`#1c2838`). The card header includes a top-right corner chamfer, an active status beacon (green/amber dot), and uppercase metadata (`SEC-ID // 084`). Content inside uses `Geist` body copy with highlighted technical specs in `JetBrains Mono`.

### Input Fields & Frequency Tuners
- **Text Inputs:** Monospaced inputs with an integrated left-side prompt marker (`>_`). Background set to `#07090e`, border `#1c2838`, focusing to `#00f3ff` accompanied by a blinking block cursor.
- **Frequency Slider / Depth Scrubbers:** High-density horizontal track with graduated tick marks every 5% increment. Active thumb is a vertical cyan reticle displaying realtime numeric frequency readout hovering above it.

### Telemetry Readouts, Chips & Indicators
- **Status Chips:** Miniature rectangular tags featuring a subtle `#00f3ff15` fill, `1px solid #00f3ff40` border, and `label-micro` typography.
- **Depth Gauge Bar:** Vertical bar anchored to viewport margins with real-time pressure ratings, oxygen saturation levels, and bathymetric zone labels (Bathyal, Abyssal, Hadal).

### Audio Waveform & Glitch Stream
- Real-time animated canvas element visualizing rogue transmissions. Audio spectral ribbons modulate between `#00f3ff` (coherent telemetry) and `#a855f7` (anomalous interception) with CRT scanline post-processing.