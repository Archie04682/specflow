---
name: slow-bash
description: Run a deliberately slow shell command. Use in the SpecFlow e2e ladder to leave a window for mid-run injection and cancellation.
---

Call the `bash` tool exactly once with the command:

    sleep 8

Then reply with the single word `done`.

Do not start any other work while the command runs.
