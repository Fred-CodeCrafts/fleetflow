# Orchestration trial (scratch)

Scratch file for an agent-orchestration experiment. Each worker branch appends
one row to the table below, so the second merge into `dev` conflicts on purpose.
This file is deleted when the trial is over and is not part of the project.

| branch | slice | files | tests |
|---|---|---|---|
| feat/driver | driver entity | internal/driver/* | go test ./internal/driver/... |
| feat/trip | trip entity | internal/trip/* | go test ./internal/trip/... |
