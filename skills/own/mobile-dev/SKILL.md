---
name: mobile-dev
description: "Build and check a React Native or Expo change: navigation, platform differences, performance, tests and a device or simulator check. Use when the project depends on react-native or expo."
applies_to: { deps: [react-native, expo] }
roles: [builder, verifier]
---

# Mobile development (React Native)

Only React Native and Expo are covered. Native Android or iOS projects are not (design section 23).

## Before you code

1. Read `app.json` or `app.config.*`, the navigation setup and how the project talks to native modules. Match them.
2. Check whether the project uses Expo managed workflow, a dev client or bare React Native. A native module change needs a new dev client build; say so in the spec instead of assuming it works over the air.

## While you code

- Use the platform's own patterns for navigation, lists (`FlatList` with stable keys, not `ScrollView` for long lists) and safe areas. Test the behaviour on both iOS and Android when the code branches on `Platform`.
- Keep work off the JS thread when it is heavy; avoid inline functions and objects in list items that re-render often.
- Ask for permissions at the moment of use, with a reason, and handle a refusal.
- Store secrets in the platform's secure storage, never in the bundle or in `AsyncStorage`. Treat deep links and push payloads as untrusted input.
- Do not add a native dependency without `build-vs-buy`: it changes the build and review cost.

## Check

1. Run the project's `verify` command: lint, types, unit tests.
2. Add or update a test with the tooling the project has (Jest with React Native Testing Library, Maestro or Detox flows).
3. If a simulator or device is available, run the changed screen and report what you saw. Otherwise state that the check was not possible; never claim a run you did not do.
