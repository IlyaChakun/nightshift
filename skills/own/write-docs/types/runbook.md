# Runbook, setup, how-to or reference page

A how-to gives the steps for one goal; a reference page lists facts to look up ([Diátaxis](https://diataxis.fr/), read 2026-11-26). A runbook is a file of how-to sections for operating a system.

Budget: one task about 150 words plus its commands; a file at most 4,000 words (reference page 3,000). Our numbers: a section an agent loads should fit in one chunk.

## Shape of one task

1. Heading: the goal as a verb phrase (`Restore a backup`).
2. When to use it, one line, with the symptom or error text in backticks.
3. Numbered steps, each one command or action, conditions first.
4. How to check it worked: the command and the expected output.

## Keep it short

- One task per section; a shared step is its own section and the others link it.
- Explanations of why go to the design; facts like ports and keys live in one reference table.
- Over the max: split the file by topic (install, operate, recover) and link the parts.
