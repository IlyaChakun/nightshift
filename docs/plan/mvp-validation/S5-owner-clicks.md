# Ask the owner only for gates not yet proven

State: proposed

Source: the day audit of 2026-11-28 in the project chat and the owner's messages in the Mac thread at 19:37, 19:46 and 20:12 UTC. Of about ten owner clicks that evening, three tested a gate for the first time. The Mac thread followed "What the owner does by hand" in this folder's `README.md`, which asks for a GitHub Approve in both V4 and V5. It also undid the owner's "turn review off in the test project" after V5, and asked for a Merge again.

Proposed edits to `README.md` for the session that owns it:

1. In "What the owner does by hand", items 3 and 4: the owner clicks only for the first live pass of each human gate (spec Approve, PR Approve, PR Merge). V4 proved all three on 2026-11-28 (tasks #1010 and #1011). Later rounds run the sandbox tenant without required review (`docs/decisions.md`, "Taken", 2026-11-28).
2. Name `.agents/skills/live-validation` under "Rules for this stream" as the skill the Mac thread uses before it asks the owner for anything.

Done when: the README carries both edits, and the next validation round asks the owner for no click on a gate that has already passed.
