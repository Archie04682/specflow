---
name: write-file
description: Write a deterministic proof file for the SpecFlow e2e ladder. Use when a test needs a known file change and a known tool call sequence.
---

Call the `write` tool exactly once with these exact arguments:

- path: `proof.txt` (lowercase, `.txt`; never `PROOF.md` or another name)
- content: `written by pi`

Then reply with the single word `done`.

Do not use any other tool. Do not read the file back. Do not rename the file.
