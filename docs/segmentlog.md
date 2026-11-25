# Segment log

`shared/segmentlog`. An append-only log of records divided into segment files. A record is a payload and a segment has a header with a meta, and the package looks into neither: whoever uses the log decides what is in them (an engine, a seed). Each record has an ID that the log counts (below). It imports only `shared/logging` and `shared/atomicfile`.

## Files

One directory for each log. A segment is `seg-<index>.pbsl`, the index in 20 digits counted from 0 in the order the segments were made, so the names sort in that order.

| Part | Layout |
|---|---|
| header | magic `PBSL`, format version `u16`, begin `u64`, meta length `u32`, meta, crc32 of all before it |
| record | payload length `u32`, crc32 of the ID of the record (not written), the length and the payload, payload |
| commit | `0xFFFFFFFF` `u32`, crc32 of the count, count `u32` (the records since the last commit) |

- Numbers are little-endian, the crc is CRC-32C. The crc of a record covers its ID, which is not in the file: the reader works it out from the `begin` of the segment and the records before, so a record that is at another place than where it was written (a batch that is gone, records that were swapped or put back) does not match and is an error, or a cut at the end of the last segment.
- The meta is at most 64 KiB; the user of the log writes in it what is the same for every segment of the log (an engine, a seed).
- A payload is at most 16 MiB.

## Record IDs

| What | Rule |
|---|---|
| the ID of a record | a number of `u64` that the log counts: the first record of a segment has the ID of the begin in its header, and each record after it the next number, across segments too, so the IDs of a log have no gap |
| who gives it | the log, in `Append`: the first record of an empty log has the ID 1, and each record after it the next number; a log that was there goes on after its last record, which `Open` finds in the last segment |
| what it is for | the user of the log can make it mean something (the tick of a frame), and then need not write it in the payload: a reader asks for a record by its ID, and the segment is found from the begins in the headers |

## Writer

| Method | Result |
|---|---|
| `Open(dir, Options)` | creates `dir`, repairs the last segment (see below), starts the goroutine that writes; `ErrBadOptions` for options that cannot work |
| `Append(payload)` | adds a record to the buffer of the batch and returns its ID |
| `LastID()` | the ID of the last record, appended or in the log when it was opened; 0 when there is none |
| `Commit()` | hands the batch to the goroutine that writes; returns the first error of the log, if any |
| `Close()` | commits what is pending, waits until everything is written and synced; later calls return `ErrClosed` |
| `Err()` | the first error of writing or syncing, or nil |
| `Stalled()` | how many `Commit` calls had to wait for the goroutine that writes |
| `Options.OnSync` | `func()`, called by the goroutine that writes, after records were forced to disk (only when there were some), so that whoever waits for them need not look again and again; `Synced()` says which |
| `Synced()` | the ID of the last record that is on the disk (written and forced), or 0; what was in the log when it was opened counts; behind the records handed over by at most what one `SyncEvery` holds; it stops where it was when the writer has failed |

| Option | Meaning |
|---|---|
| `SegmentBytes` | a segment is closed when it reaches this size, at the end of a batch |
| `SyncEvery` | period of the sync to disk (group commit) |
| `Backlog` | batches that can wait for the goroutine that writes; at least 1 |
| `Meta` | the meta that is written in the header of every new segment; nil is an empty meta; a longer one than 64 KiB is cut |

- `Append` and `Commit` do no I/O: the file is written, and synced every `SyncEvery`, by the goroutine of the Writer. A `Commit` waits only when `Backlog` batches are already waiting: a log does not drop records.
- A batch (what one `Commit` hands over) is its records and then a commit, written with one `Write`; a segment is never split inside a batch.
- A batch is all or nothing: a `Write` can be cut by a crash or by a process that is killed (the kernel copies a large `Write` in pages, and a kill stops it between two of them), which leaves part of a batch. The commit that is missing, or whose count is not the records before it, says so, and the batch is not read and is cut from the file.
- After the first write or sync error nothing more is written, and the error is returned by `Commit` and `Close`; it is also logged.
- A new segment is made with `O_EXCL`, at the index after the last one; a segment of that name that already exists is an error.
- The methods can be called from any goroutine, one at a time.

## Reader

| Method | Result |
|---|---|
| `OpenReader(dir, from)` | starts at the segment that holds the record with the ID `from`, and reads on to the end of the log |
| `Next()` | moves to the next record, which has an ID of `from` or more; false at the end or on an error |
| `ID()`, `Payload()` | of the current record; the payload is valid until the next `Next` |
| `Meta()`, `SegmentIndex()` | of the segment of the current record |
| `LastFrameID(dir)` | the ID of the last record of the log, or 0 when there is none; it reads the last segment that has a whole batch and no other |
| `Err()` | the error that stopped `Next`, or nil |
| `Segments(dir)` | the segments in the order they were made |

- The reader starts at the last segment whose begin is at or before `from`, or at the first when none is: a binary search over the headers of about log2(n) of the n segments, since the begins grow with the index. The records of that segment before `from` are read and not given.
- A segment must begin where the one before it ended: its `begin` is the ID after the last frame of that segment, and when it is not the reader stops with an error that wraps `ErrCorrupt` (a segment that is gone, or one that was put in the wrong place). A read that starts after the hole does not see it, since it does not read the segments before its own.
- The last segment is read up to the end of its last whole batch: a record cut off, a commit that is missing or wrong, and the records before it are the end of the log, not an error. Anywhere else it is an error that wraps `ErrCorrupt`.
- A last segment whose header was not finished when a crash came is not in the log.

## Cut

| Call | What it does |
|---|---|
| `CutAfter(dir, limit, archiveDir)` | cuts the log after the record with the ID `limit`: the records with a greater ID go to `archiveDir`; the log must not be open for writing, and the ID of the next record of a Writer opened on what is left is `limit + 1` |

- The segment that holds the record after `limit` is found like the one of `OpenReader` (the headers of about log2(n) segments) and is the only one that is read. It is cut at the start of the batch of that record, which takes with it the records of the batch that were to be kept; the segments after it are taken out of the log, from the last, so that a crash in the middle leaves a log that is whole and longer than it should be. Nothing is thrown away: the bytes that were cut are in `<archiveDir>/<segment>.discarded-<offset>`, a segment taken out whole is moved there as `<segment>.discarded-0`, where `Segments` does not see it, and a warning says so. `archiveDir` is made when something is cut, and is on the same volume as `dir`.
- A log with no record after `limit` is not changed, a `limit` before the first record cuts all of the log, and a log that does not exist is empty.

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
