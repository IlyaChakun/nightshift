# Run L1 and L2 on the revert pull request

State: proposed

Source: P6c open point; design section 7 asks for L1 and L2 on every change, and the revert pull request the conductor opens after a rollback merges on code-host checks alone.

Why it needs a new job: L1 is `nightshift check` in a worker before a push, run by a role job. A revert made by the conductor has no role job, so there is nothing that runs L1 on it. A model-free check job in the worker (checkout of the revert head, `nightshift check`, the stack's L2 tests, a JSON result, no harness) would close it.

What to do: a `check` job kind in `jobsapi` and the runner with no harness; the conductor queues it for the revert head and merges the revert only after it passes and the code-host checks are green; a failure keeps the train's hold and posts one blocker card.

Check: T4 on the twins: a revert whose check job fails is not merged and the hold stays.

Open: whether the same job should also re-run L1 after a train rebase (today only code-host CI runs there).
