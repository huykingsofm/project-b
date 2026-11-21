# R4. Generic base until the tag `base-v1` (hard)

The core is a generic multiplayer game base: state, actions, plan, effects, engines, zones, zonelayers. Until the base is tagged `base-v1`, the mechanisms, names and config variables of the core carry none of this game's own business (cultivation, realms, spiritual roots, sects, techniques, tribulation, talismans, territory wars). The business enters after the tag, through the extension points the base offers (effects, actions, content, config variables), not by editing the base.

**Examples are allowed.** Sample game content may be added to the base as an example, such as an item, a monster, a stat, a skill or an action like picking up an item. An example:

- uses neutral names, and says in its comment or its file that it is an example;
- lives in a place of its own (a sample content package, a sample action or entry), so the mechanism around it does not depend on it;
- can be replaced or removed without changing the mechanism, because a game built on the base replaces the examples with its own.

Examples exist to show early where a design does not fit, and to stay as models for later work.

When the base is complete, a semantic review of every package finds what still carries business meaning outside the examples, and only then is the tag made.
