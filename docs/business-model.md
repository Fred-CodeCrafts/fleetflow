# FleetFlow — business model

Status: a proposal. Prices, market ranges and margins below are planning assumptions
and are labelled as such; the infrastructure figure is a list-price estimate, not a bill.

## 1. Who pays, and for what job

Mid-size Jakarta and Jabodetabek delivery fleets, roughly 20 to 200 vehicles: big enough
that a spreadsheet has stopped working, small enough that nobody has bought an
enterprise transport management system.

Three people in that company feel the problem, and they are the same buyer at different
moments. The **fleet manager** assigns vehicles every morning and pays for the wrong
choice in overtime and fuel. The **owner** watches the fuel line. The **person who
answers questionnaires** — to a customer, a financier, or a sustainability report — has
to produce a carbon number per delivery and currently has nothing defensible to send.

The job FleetFlow is hired for is narrow and repeatable: assign the vehicles, keep the
history, and print a carbon figure that names the model behind it.

## 2. What is sold

The console is Apache-2.0, so the software is not the product. What is sold is the
hosted instance, the Jakarta road data, the report, and support.

| Tier | What the buyer gets | Price (proposed) |
|---|---|---|
| Ledger | emissions per trip and a monthly report, no dispatch | Rp 25,000 / vehicle / month |
| Console | dispatch, trip history, live map, carbon ledger | Rp 100,000 / vehicle / month |
| Onboarding | one-off: data import, hub and tariff setup, driver training | Rp 5,000,000 / fleet |

The Console price sits deliberately *below* a GPS subscription, which in Indonesia is
commonly quoted around Rp 150,000 to Rp 400,000 per vehicle per month. FleetFlow reads
the positions a fleet already collects; it does not sell the hardware and it must never
be the largest line on a fleet's software bill.

## 3. Unit economics, with the arithmetic shown

A 100-vehicle fleet on Console is Rp 10,000,000 a month. At about Rp 16,000 to the
dollar that is roughly USD 625.

Hosting for it is one small managed PostgreSQL instance and one container. On Cloud Run
or Render that is under USD 150 a month at list price, about Rp 2,400,000 — a **76% gross
margin before support**.

There is no per-request map licence to pay. OSRM is BSD-2-Clause, SUMO is EPL-2.0, and
OpenStreetMap data is ODbL, free to use with attribution. That absence of a metered
mapping bill is the structural reason a small team can offer this at all; a product built
on a per-route commercial API could not.

The cost that actually decides viability is support, not compute. At three hours per
fleet per month and a loaded cost of Rp 200,000 an hour, support is Rp 600,000 against
Rp 10,000,000 of revenue — 6%. At ten vehicles the same three hours is 60% and the model
collapses. **That is the argument for selling to mid-size fleets rather than to
five-vehicle operators**, and for pricing onboarding separately so the first month is
paid for.

## 4. Go to market

**Channel first.** Telematics installers and resellers already have the vehicles, the
hardware and the GPS feed. FleetFlow's ingestion is one authenticated POST per position,
so a partner platform can point at it without changing their product. A referral fee or
revenue share reaches fleets that a solo developer cannot visit one by one.

**Wedge: the carbon ledger.** It is the cheapest thing to sell, because it answers a
reporting deadline and requires no change to how anyone dispatches. Dispatch is the
expansion, sold to the same account once the report has been trusted for a quarter.

**Credibility asset.** Settings → Routing Lab reproduces a routing experiment from a
clone, with the control case included. In a market where routing vendors show a
percentage with no method, a demo that prints its own exclusions is the sales argument.

## 5. Competition

- **Telematics vendors** are strong on where a vehicle is, and weak on what a trip meant:
  no per-trip carbon arithmetic, no provenance on the numbers.
- **Route optimisers** sell a route, and are often opaque about which model produced the
  distance or the duration. FleetFlow prints `OSRM`, or a 503.
- **Spreadsheets** are free and are the real competitor. Nothing here beats a spreadsheet
  on price; it beats it on the hour spent reconciling three files at month end.

The moat is not the algorithm — the dynamic-weights idea is from a published thesis
proposal. It is the Jakarta road data, the hosted instance, the audit trail that survives
a customer's question, and the monthly report.

## 6. What must be true

1. Target fleets already pay for GPS hardware and a position feed.
2. Someone with authority is asking for a carbon number per delivery — a customer
   questionnaire, a tender, or a report to a financier.
3. Support stays near three hours per fleet per month once onboarding is self-serve.
4. The free Apache-2.0 core does not cannibalise the paid tier: the paid part must stay
   operationally annoying to self-host, not artificially crippled.

If assumption 1 or 2 is false in a given segment, FleetFlow is a tool, not a business,
and the honest move is to sell the report rather than the console.
