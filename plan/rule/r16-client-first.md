# R16. Client first (hard)

It applies only to what needs low latency, what the player feels at once: moving, a change of speed, a dash, the start of an attack or a skill and its cooldown, one's own chat message.

1. The rule lives on the client: it holds the numbers and the formula (code shared with the server or a table of test vectors, and the numbers the server publishes to it) and applies the rule itself, acting at once.
2. The server checks the same rule and decides what is real.
3. The reply or the state the server sends only confirms, or says how far the client is from the rule so that the client corrects itself.

Everything else waits for the server: signing in, moving to another zonelayer or zone, loading, outcomes with chance or money or items in them, the market. **While the client waits it does nothing:** it sends no input and stands still, until the server has answered and, after a move to another place, until the client has sent `Ready`.

**The server is optimistic in the same way:** the game cares about time more than about exactness to the tick. The engines (world, zones, zonelayers) never wait for each other; each publishes its latest state, and another engine reads the latest it has, not the state of a particular tick. Only the engine that owns a piece of data decides exactly what it is; a decision that needs the data of another engine is made at once on what is known, and corrected later if the owner says otherwise.
