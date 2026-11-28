# The evidence card shows the screenshots, not their paths

State: proposed

Source: V4 rerun on the Mac, 2026-11-28 (task #1009, by the Mac thread's account, not checked here): the evidence and decision cards list paths such as `artifacts/23/…/evidence/01.png`; the owner cannot open them from Slack.

Next step: the Slack adapter uploads each listed image into the card's thread (`files.uploadV2`; the app manifest already has `files:write`, `deploy/slack-manifest.yaml`), reading the files from the artifact store through the outbox, with a twin test. A *should* item: the paths stay as the fallback, and the MVP does not wait for it.
