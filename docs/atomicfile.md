# Atomic file

`shared/atomicfile`. Writes a file so that it is on the disk whole or not there at all, and syncs directories. It imports nothing of the project.

## Entry points

| Call | What it does |
|---|---|
| `atomicfile.Write(path, data)` | makes the directory of `path` when it is not there, writes `path + ".tmp"`, syncs it, renames it to `path`, and syncs the directory; a file already at `path` is replaced whole |
| `atomicfile.SyncDir(dir)` | forces the entries of a directory to disk, so that a file made, renamed or deleted in it is there after a crash |

- A temporary file that a crash left is not the file; the next `Write` of the same path writes over it.
- Used by the checkpoint files ([checkpoint](checkpoint.md)), the cache of the recovery line ([recovery](recovery.md)) and the segment log ([segmentlog](segmentlog.md)).
