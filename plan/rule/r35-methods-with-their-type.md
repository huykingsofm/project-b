# R35. The methods of a type are in the file of the type, right below it (hard)

A type and its methods are written in one file, one after the other: the `type` declaration, its constructor and then every method of it, with nothing of another type between them. A method is never put in another file of the package, and never far from its type behind the declaration of another type, whatever the reason (to keep a file short, to group what the methods do, such as encoding or hashing).

- **Allowed:** a function that is not a method (`DecodeState(r)`, a helper) in any file; it is put right after the methods of the type it reads or makes (`decodeCellData` after `cellData.Encode`). The tests of a type in `<file>_test.go`.
- **A type that is too big for one file** is split into smaller types, each with its file and its methods, and not one type over many files.
- **Not allowed:**
  - `func (s *State) Encode` in `codec.go` and `func (s *State) Hash` in `hash.go` while `type State` is in `state.go`.
  - `type cell`, then `type cellData`, then the methods of `cell`: the methods of `cell` are below `cell`, and `cellData` has its own place with its own methods right below it.

Why: a reader who finds a type finds all it does at once, below it, and a method that is somewhere else is easy to miss, to write twice, and to leave behind when the type changes.
