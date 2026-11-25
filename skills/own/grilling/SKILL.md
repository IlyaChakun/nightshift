---
name: grilling
description: Question an idea one or two questions at a time until the goal, scope and acceptance criteria are clear. Use when the owner's idea is raw or ambiguous and the repository and the book do not answer it.
---

# Grilling

Ask only what you cannot find yourself. The owner's time is the scarcest thing in the factory.

## Method

1. **Look first.** Read `AGENTS.md`, `docs/`, ADRs and the code the idea touches. The book search (`ns search`) is not built yet; never say you searched the book. An existing decision ends the question: quote it with its source and date.
2. **Name what is unclear.** Goal, who benefits, how we will know it works, scope, what is out of scope, risks. Pick the one or two gaps that change the design most.
3. **Ask one message at a time, one or two questions.** Each has two or three options and your recommendation with a reason. Never an open "what do you want?".
4. **Take the answer as data.** Update your picture, drop the gap, and move to the next only if it still matters.
5. **Stop when you can write the spec.** More questions after that cost the owner time and add no information.

## Do not

- Ask what the repository already answers, or ask the same thing twice in a thread.
- Stack five questions in one message.
- Agree to be polite. If the idea is weak or contradicts an earlier decision, say so with the source.
