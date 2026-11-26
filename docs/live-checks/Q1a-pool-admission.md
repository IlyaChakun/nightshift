# Hold jobs for a capped credit pool

See [Q1a](../plan/phase-5/Q1a-monthly-credit-pools.md). On a disposable stack, give a fake provider a finite pool and leave another fake provider uncapped. Queue one job on each, restart the service with both jobs waiting, then claim work. Record the held job's ID, state and attempt count and the unrelated job's claim. Only the unrelated job should receive a lease. This check needs no paid call.

Status: pending
