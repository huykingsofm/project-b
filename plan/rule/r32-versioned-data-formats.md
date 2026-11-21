# R32. A change to how data is encoded makes a new version, never overwrites the old (hard, after pre-alpha)

Data that is written and read back later (the replay log, a checkpoint, anything with an `Encode` and a `Decode`) has two versions, each with its own owner:

- **The version of the format of the container**, such as the `segmentlog` version in the header of a segment. It describes how the library frames bytes (header, records, commits) and nothing about what they mean. Only the library changes it, and it reads every version it has ever written.
- **The version of the data**, such as the format version in the meta of a segment of the replay log. It describes how the data of the business is encoded: commands, actions, characters, items. It is written by the user of the container, and the reader passes it to `Decode`, so every `Decode` knows which version of the bytes it is reading. A data type that is inside another (a `Character` in a command) is read by the version of the whole, not by one of its own.

A change that breaks the bytes of data that has an `Encode` (a field added, removed, moved or retyped, a meaning changed, an ID reused) makes a new version of the data:

- The new `Encode` writes the new version only.
- `Decode` keeps the code that reads each older version that can still be on disk, chosen by the version it is given. The old code is deleted only when no log or checkpoint of that version can be read any more (for example, after a checkpoint of the new version has been taken and the older segments are deleted).
- The change is never made by overwriting the old `Encode` and `Decode`, because every file written before it would then be read wrongly, or not at all.
- A change that only adds (a new command with a new ID at the end of its register) breaks nothing and needs no new version.

**Pre-alpha (the stage the project is in now):** the rule above does not apply, and this does, to every breaking change in the code (a public API, a name, an interface, a signature, a configuration, a data format, the bytes of a log or a checkpoint), not only to written data. A breaking change is made as it is and everything that depends on it is refactored, as widely as that takes. There is no new version, no deprecated copy kept beside the new one, no adapter or alias for the old shape, no code that reads an older format and no back-compatibility of any kind, anywhere: whatever was written before the change (a log, a checkpoint) is thrown away. Pre-alpha is the stage of development in which there is no official production. It is not a fixed phase: it lasts until the owner says it is over, and the owner records that in the plan. From then on this rule applies in full.

Why: a log or a checkpoint outlives the code that wrote it, and a replay that reads old bytes with a new reader gives a wrong state without any error.
