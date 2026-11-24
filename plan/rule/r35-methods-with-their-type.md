# R35. The methods of a type are in the file of the type (hard)

A type and its methods are written in one file: the `type` declaration, its constructor and every method of it. A method is never put in another file of the package, whatever the reason (to keep a file short, to group what the methods do, such as encoding or hashing).

- **Allowed:** a function that is not a method (`DecodeState(r)`, a helper) in any file, and the tests of a type in `<file>_test.go`.
- **A type that is too big for one file** is split into smaller types, each with its file and its methods, and not one type over many files.
- **Not allowed:** `func (s *State) Encode` in `codec.go` and `func (s *State) Hash` in `hash.go` while `type State` is in `state.go`.

Why: a reader who finds a type finds all it does in the one file, and a method that is somewhere else is easy to miss, to write twice, and to leave behind when the type changes.
