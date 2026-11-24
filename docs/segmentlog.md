# Segment log

`shared/segmentlog`. An append-only log of records divided into segment files. A record is a payload and a segment has a header with a meta, and the package looks into neither: whoever uses the log decides what is in them (a tick, an engine, a seed). It imports only `shared/logging`.

## Files

One directory for each log. A segment is `seg-<index>.log`, the index in 20 digits counted from 0 in the order the segments were made, so the names sort in that order.

| Part | Layout |
|---|---|
| header | magic `PBSL`, format version `u16`, meta length `u32`, meta, crc32 of all before it |
| record | payload length `u32`, crc32 of the payload, payload |
| commit | `0xFFFFFFFF` `u32`, crc32 of the count, count `u32` (the records since the last commit) |

- Numbers are little-endian, the crc is CRC-32C.
- The meta is at most 64 KiB; the user of the log writes in it what lets a reader choose a segment without reading its records.
- A payload is at most 16 MiB.

## Writer

| Method | Result |
|---|---|
| `Open(dir, Options)` | creates `dir`, repairs the last segment (see below), starts the goroutine that writes; `ErrBadOptions` for options that cannot work |
| `Append(payload)` | adds a record to the buffer of the batch |
| `Commit()` | hands the batch to the goroutine that writes; returns the first error of the log, if any |
| `Close()` | commits what is pending, waits until everything is written and synced; later calls return `ErrClosed` |
| `Err()` | the first error of writing or syncing, or nil |
| `Stalled()` | how many `Commit` calls had to wait for the goroutine that writes |
| `Options.OnSync` | called by the goroutine that writes, after batches were forced to disk, with how many batches that sync put on the disk (only when there were some), so that whoever waits for them need not look again and again |
| `Synced()` | how many of the batches that `Commit` handed over are on the disk (written and forced); a `Commit` with no record appended since the last one is not a batch; behind the batches handed over by at most what one `SyncEvery` holds; it counts from the batch this writer was opened at, and it stops where it was when the writer has failed |

| Option | Meaning |
|---|---|
| `SegmentBytes` | a segment is closed when it reaches this size, at the end of a batch |
| `SyncEvery` | period of the sync to disk (group commit) |
| `Backlog` | batches that can wait for the goroutine that writes; at least 1 |
| `SegmentMeta` | `func(first []byte) []byte`: the meta of a new segment, given the first payload it will hold; nil is an empty meta |

- `Append` and `Commit` do no I/O: the file is written, and synced every `SyncEvery`, by the goroutine of the Writer. A `Commit` waits only when `Backlog` batches are already waiting: a log does not drop records.
- `SegmentMeta` is called by the goroutine of the Writer, not the caller's, so it must not use what the caller changes.
- A batch (what one `Commit` hands over) is its records and then a commit, written with one `Write`; a segment is never split inside a batch.
- A batch is all or nothing: a `Write` can be cut by a crash or by a process that is killed (the kernel copies a large `Write` in pages, and a kill stops it between two of them), which leaves part of a batch. The commit that is missing, or whose count is not the records before it, says so, and the batch is not read and is cut from the file.
- After the first write or sync error nothing more is written, and the error is returned by `Commit` and `Close`; it is also logged.
- A new segment is made with `O_EXCL`, at the index after the last one; a segment of that name that already exists is an error.
- The methods can be called from any goroutine, one at a time.

## Reader

| Method | Result |
|---|---|
| `OpenReader(dir, start)` | starts at the segment `start` chooses, and reads on to the end of the log |
| `Next()` | moves to the next record; false at the end or on an error |
| `Payload()` | the payload; valid until the next `Next` |
| `Meta()`, `SegmentIndex()` | of the segment of the current record |
| `Err()` | the error that stopped `Next`, or nil |
| `Segments(dir)` | the segments in the order they were made |

- `start` is `func(meta []byte) bool`: whether a segment is at or before where the caller wants to start. The reader starts at the last segment for which it is true, or at the first when it is true for none. It must be true for the first segments and false for the rest; it is called for about log2(n) of the n segments. A nil `start` reads from the first segment.
- The records of the first segment that come before what the caller wants are given too; the caller skips them.
- The last segment is read up to the end of its last whole batch: a record cut off, a commit that is missing or wrong, and the records before it are the end of the log, not an error. Anywhere else it is an error that wraps `ErrCorrupt`.
- A last segment whose header was not finished when a crash came is not in the log.

## Cut

| Call | What it does |
|---|---|
| `CutAfter(dir, keep)` | cuts the log after the last batch whose records `keep` accepts: the first record that `keep` refuses, and all after it, go; the log must not be open for writing |

- The segment that holds the first refused record is cut at the start of its batch; the segments after it are taken out of the log. Nothing is thrown away: the bytes that were cut are in `<segment>.discarded-<offset>`, a segment taken out whole is renamed `<segment>.discarded-0`, which `Segments` does not see, and a warning says so.
- A log in which `keep` refuses nothing is not changed, and a log that does not exist is empty.

## Repair

`Open` makes the last segment whole after a crash; the others are not touched.

| Last segment | Open does |
|---|---|
| what follows the last whole batch (a record cut off, or records with no commit) | cuts the file after the last commit, and syncs |
| no whole batch | deletes it |
| header cut off (never finished) | deletes it |
| header whole but wrong (magic, version or crc) | leaves it and `Open` returns an error that names the file |

- What `Open` cuts off is not thrown away: the bytes are copied to `<segment>.discarded-<offset>` and synced before the segment is cut, and a warning is logged. A torn batch and damage in the middle of the segment look the same, so a person decides.
- A new segment, a deleted one and a copy of discarded bytes are followed by a sync of the directory, so the name survives a crash as the bytes do.
- The Writer then starts a new segment; it never appends to an old one.
- One process at a time may open a directory. Nothing stops two (see the plan: a directory lock when several processes run).

Benchmarks: [segmentlog](benchmark/segmentlog.md).
