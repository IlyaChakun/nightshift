# Evidence card after a real acceptance

The [phase 6 P5b item](../plan/phase-6/P5b-deployments-card-evidence.md) posts a "Ready for acceptance" card when the Verifier's acceptance passes: the preview URL, the screenshots and video the acceptance job left, and the job's duration and cost.

Run it in the MVP validation, stage V4 (`docs/plan/mvp-validation/README.md`), after V0 to V3 pass: one idea in Russian in Slack, through a passing acceptance on a `compose-local` preview. Expect: one card in the task's thread with Russian labels, the preview URL that the Verifier opened, the screenshot and video paths `artifacts/<job>/<attempt>/evidence/NN.<ext>`, and those files on the artifacts volume, a non-zero duration and the job's cost; no scenario text and no file other than images and video on the card; no second card on later ticks.

Not covered by the T4 test: Slack rendering, whether the paths help the owner find the files on the Mac, and the cost a real harness reports.

Run, 2026-11-28 (by the Mac thread's account; evidence `~/nightshift-evals/2026-11-28/V4-A-1010.txt` and `V4-B-1011.txt` on the Mac, not read by the thread that recorded it): the card reached the owner in the V4 runs (tasks #1010 and #1011).

Status: passed 2026-11-28 in V4 (tasks #1010, #1011)
