# Judge Q&A rehearsal

Five questions the review said will be asked, with the answer the repository now
supports. Rehearse the honest answers; do not improvise a claim the repo cannot
show on screen.

---

## Q1. "Walk me through the arithmetic behind the grams-per-kilometre figure."

Open `/sustainability` and point at the class column.

> Every trip is charged against the class of the vehicle that drove it, and the
> class travels with the number. A 14 t diesel box truck burns 0.30 kg of diesel
> per kilometre at 3.16 kg CO2 per kg, so 948 g/km. A 4.5 t truck burns 0.18, so
> 569. A diesel van 0.11, so 348. A petrol van 0.09 at 3.10, so 279. The electric
> cargo motorcycle burns nothing at the tailpipe, so zero.
>
> These are planning assumptions for congested Jakarta delivery work, not
> certified values and not measured from an operator. The page prints the model
> name with the figures for that reason. The test that a diesel truck and an
> electric motorcycle differ on the same distance is
> `internal/fleet/fleet_test.go`.

If asked "why should I trust the factor": don't. Say it is an assumption on the
record, and that the next step is replacing it with the operator's own fuel
bills.

---

## Q2. "Which of the numbers in this demo were actually produced by SUMO?"

> None of the stored runs. They are the synthetic runner, and every result says
> so in a `runner` field: `runner: "synthetic"`, `sumo_version` empty. The run
> panel prints "FleetFlow synthetic generator", not a SUMO version.
>
> The SUMO path exists and is wired: `internal/simulation/sumo` writes the
> routes, runs `duarouter` and `sumo`, and parses tripinfo, fcd and emission
> output with the HBEFA3 class. I have not produced a validated Jakarta SUMO run
> on this machine, so I am not going to claim one. What I will show you is the
> code path and the runner field that keeps that claim honest.

Do not show a synthetic run and call it SUMO. If there is time before the
showcase, run one 20-vehicle SUMO scenario and show `runner: "sumo"` beside it.

---

## Q3. "Your design document forbids fallback data. Your main wired a fallback router."

> You are right, and it is gone. `internal/osrm/fallback.go` fabricated geometry
> when OSRM failed. It leaked in as a development convenience.
>
> Now `/api/route` returns 503 naming OSRM, and `/readyz` reports
> `routing: "unavailable"` until OSRM answers. `TestReadyzReportsRoutingUnavailable`
> and `TestRouteSurfacesRoutingServiceFailure` are the tests. The commit is
> `fix(api): report routing outages instead of inventing a route`.

Then: stop OSRM, reload the map, let them watch the page say so. This is the
cheapest credibility in the demo.

---

## Q4. "What is your evidence this helps a Jakarta fleet, in money?"

> I have mechanism evidence, not field evidence. On a synthetic 36-node grid with
> a zero-share control, routing on own observations cut mean trip time by 20.21
> seconds across ten seeds, 9 wins and 1 loss, and the effect is exactly zero
> when no vehicle reports. That is in `docs/results.md`, recorded by the study
> script that has since been removed from the tree: the figures stand, but I
> cannot hand you the command that regenerates them today.
>
> I cannot convert it to rupiah. I have no operator, no measured Jakarta speeds
> and no fuel bills, so any rupiah figure I gave you would be invented. What I
> can give you is the mechanism, the seeds it ran on, and the boundary.

Do not add a made-up cost saving. The admission is the answer.

---

## Q5. "Why does the branch I clone differ from the branch you demo?"

> It should not, and it does not now. `feat/fleet-ops-refinements` is merged into
> `dev`, `dev` into `main`, and `v0.1.0` is tagged on that merge, so `git log
> main` is what you are looking at.
>
> The deployed Cloud Run revision is **not** on that tag, and I will not pretend
> otherwise: the project's billing account closed on 24 October, so the last
> revision predates this work and does not start. `deploy/deploy.ps1` is the one
> command that puts the tag's image on that service once billing is restored.

---

## Bonus. "Your writes are unauthenticated. What stops me scripting your booth?"

> A rate limit of 100 requests per second with a burst of 200, and a 64 KiB body
> cap, so no single request or flood can wedge the process. That is availability,
> not authorisation. Operator writes should require the same key as GPS
> ingestion, and today they do not; it is in the open issues rather than in a
> claim that this is safe.
