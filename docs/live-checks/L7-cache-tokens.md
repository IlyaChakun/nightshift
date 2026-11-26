# Cache tokens per job and no CLAUDE.md on the subscription

Upgrade the stable instance to a commit with L7a. Run one Builder or Lead job on `claude-sub` on the `nightshift` tenant, then `nightshift status --task <id>`: the job line shows "N read from cache". Read the job's usage from the book and record, per role, jobs, cache reads, output tokens and `cost_usd`:

```sh
docker compose exec -T postgres psql -U postgres -d book -c "select role, count(*), sum((usage->>'cache_read_tokens')::bigint) cache_read, sum((usage->>'output_tokens')::bigint) output, round(sum((usage->>'cost_usd')::numeric), 2) cost from book.jobs where finished_at > now() - interval '7 days' group by role order by cost desc nulls last"
```

To confirm `CLAUDE_CODE_DISABLE_CLAUDE_MDS`: in the same job's transcript the first call's cache reads drop by roughly the size of `AGENTS.md` (about 2.7k tokens, estimate) against a run before the upgrade, and the role still follows the appended project instructions.

Run the `smoke` evals for `builder` and `verifier` (`evals/selection.yaml`): the tool-output line in their role files must not lower the scores, and the jobs' cache reads should drop.

The stub-binary tests cover the parsing and the environment. The real CLI, the stable book and the variable's effect need the Mac.

Status: pending
