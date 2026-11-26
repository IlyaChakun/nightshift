# Run the verification suite on macOS and Linux

State: done

The preflight for audit [#332](https://github.com/IlyaChakun/nightshift/pull/332)
found that the local suite assumed a physical checkout path, a fixed process
startup time, GNU date and a shell with mise tools on its path.

The Compose path test uses the recipe linter. The toolchain service accepts
checkouts under the configured work root and checks the resolved location
before installation. The Git cancellation test signals the child once it is
ready. The PR sweep reads UTC timestamps on both platforms, and Make runs
the tools from the pinned paths.

Check with `make verify` from the checkout with the pinned tools installed.
