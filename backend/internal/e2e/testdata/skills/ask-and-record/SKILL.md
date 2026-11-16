---
name: ask-and-record
description: Ask the operator one question and record the answer. Use in the SpecFlow e2e ladder to prove the embedded ask bridge round trip.
---

Call the `specflow_ask` tool exactly once with:

- question: "What value should be recorded?"
- kind: "input"
- title: "E2E question"

Then call the `write` tool to create `answer.txt` containing exactly the answer
you received, and reply with the single word `done`.

If the tool reports that no answer was available, write `no answer` instead.
