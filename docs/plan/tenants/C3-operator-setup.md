# C3. Check the first-instance instructions

State: done

Source: owner request to check testing, startup and setup instructions for the audit work.

The setup guide covers the chat files Compose mounts, the provider chains the wizard writes, enabling Lead and Verifier, and the expected one-shot service states. The README gives the checkout verification and drill commands.

Check with the instance wizard on a temporary output directory, load its config with `nightshift config validate`, and render the Compose file with the `obs` profile. Repeat the file preparation on a directory containing a synthetic token. Check that the file still holds that value and that a missing chat file is created with mode 0400.
