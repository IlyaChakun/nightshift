# Put the acceptance evidence on the #deployments card

State: open

Source: design section 9 ("#deployments"): the card holds the tracker issue, preview URL, PR, video and screenshots, metrics, the blast radius level with reasons and the `trace_id`. Found on 2026-11-27: the card built in `askHuman` and the "ready" publication carry no preview URL and no acceptance artifacts.

What to do: after P5a, add the preview endpoint and links to the screenshots and video the acceptance job left in its job directory (stored as artifacts, section 14) to the card; metrics are the acceptance job's duration and cost. No file of the hidden scenarios goes on the card.

Check: a golden card in a T4 scenario after P5a's passing acceptance.

Built: the runner keeps an acceptance job's screenshots and video as artifacts `evidence/NN.<ext>` (`internal/execution/runner/evidence.go`); `internal/control/dispatch/evidencecard.go` lists only those on the card, queued with the `accepted` move in `consumeAcceptance`; T4 with golden cards `testdata/golden/evidence-card-{en,ru}.txt` in `internal/control/dispatch/evidencecard_integration_test.go`. Merged, [live check pending](../../live-checks/P5b-evidence-card.md) (MVP validation V4). Not on the card yet: the tracker issue, the blast radius with reasons, the `trace_id`, and the Merge to Prod / Rollback buttons (no door handles them); the files are listed as paths, not uploaded to the chat.
