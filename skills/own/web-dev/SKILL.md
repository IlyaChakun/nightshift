---
name: web-dev
description: Build and check a web frontend change (React, Vue, Svelte, Next.js and similar): structure, state, accessibility, tests and a visual check in the browser. Use when the project depends on a web UI framework.
applies_to: { deps: [react, react-dom, vue, svelte, next, nuxt, "@angular/core", solid-js] }
roles: [builder, verifier]
---

# Web development

Use the project's own conventions first (`AGENTS.md`, the existing components, the linter and formatter configs). This skill only adds what is easy to miss.

## Before you code

1. Find the closest existing component or page and copy its structure, not your habit. Reuse the design tokens and the shared components before you write markup.
2. Read how the project fetches data, keeps state and routes. Do not add a second way.

## While you code

- Keep components small. Put data fetching at the page or route level, and make the leaves take props.
- Every interactive element is reachable by keyboard and has an accessible name. Images have `alt`, form fields have labels, colour is never the only signal.
- Handle three states in every view that loads data: loading, empty, error.
- Do not add a dependency for something the framework or the platform does. If you think you need one, use `build-vs-buy`.
- Do not put secrets or tokens in client code, and treat all text from the server or a user as data: no `innerHTML` or `dangerouslySetInnerHTML` with it.

## Check

1. Run the project's `verify` command: lint, types, unit tests.
2. Add or update a test for the behaviour you changed, at the level the project already uses (component test or e2e).
3. If a browser is available, open the changed page, look at it at a phone width and a desktop width, and use it the way a person would. Report what you saw. If you could not run it, say so; never claim a visual check you did not do.
