# R8. Choose behavior by kind with a table

Where code chooses behavior by a kind (a command type, an error, a constraint, a chat channel, a kind of change, a kind of bounce decision), use a table or registry of small units keyed by that kind, not a chain of conditionals or handlers that return whether they handled the message. A test fails when a kind has no entry. Examples: `route` by chat channel, `standing` by kind of effect, `decide` by kind of bounce decision, `infos` by action ID.
