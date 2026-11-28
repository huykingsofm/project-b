# R16. A type with its methods right below it, the order of a file, and its lines (hard)

A type and its methods are written in one file, one after the other: the `type` declaration, its constructors, then every method of it, with nothing of another type between them. A method is never put in another file of the package, and never far from its type behind the declaration of another type, whatever the reason (to keep a file short, to group what the methods do, such as encoding or hashing).

**Order of a type** (a struct, an interface, any named type): the declaration, its constructors (public first, then private), its methods (public first, then private). A constructor is a function that returns the type, whatever its name: `New…`, `new…`, `Decode…`, `Read…`.

**Order of a file**, from the top: the imports, the constants and variables (and `init`), the public functions that are not constructors, the public types, the private types, the private functions. A function is put where its meaning puts it: one that works on the whole package, or reads a file, is at the top; a private helper is at the bottom.

**Order of types:** the public ones before the private ones, and for a type that another type holds:

- a type **with no method** (a plain struct, an interface) is declared **above** the type that holds it, even when it is private and the one that holds it is public;
- a type **with methods** is declared **below** the type that holds it, below its methods.

**Inside a type:** fields of one meaning or one job are together, with a blank line between the groups; methods and functions that do alike are together. A comment is on all of the fields and methods of a group, or on none: when the names say all, there is no comment. A comment is short and says the point, not the detail.

- **Allowed:** the tests of a type in `<file>_test.go`; the test files are not covered by the order. A public name with no comment: if the linter asks for the comment, that check is turned off, and the comment is not written to please it.
- **A type that is too big for one file** is split into smaller types, each with its file and its methods, and not one type over many files.
- **Not allowed:**
  - `func (s *State) Encode` in `codec.go` and `func (s *State) Hash` in `hash.go` while `type State` is in `state.go`.
  - `type cell`, then `type cellData`, then the methods of `cell`: the methods of `cell` are below `cell`, and `cellData` has its own place with its own methods right below it.
  - a private helper in the middle of the public types, or a constructor after the methods.

Why: a reader who finds a type finds all it does at once, below it, and a method that is somewhere else is easy to miss, to write twice, and to leave behind when the type changes; and a file is read once, top to bottom, with each thing after what it needs and the private detail after the public face.

## Line length

- **The limit:** no line of Go code or of a comment is longer than 100 columns, a tab counting as 4. `lll` checks it (`make lint`, set in `.golangci.yml`), and `make ci` runs it, so a longer line fails the commit.
- **Comments are wrapped** at the limit, a paragraph to a block of lines; a list item keeps its hanging indent.
- **Code is broken at its structure:** after a comma inside the brackets of a call, a signature or a literal (one line for the arguments, or one for each when they do not fit), or after `&&` and `||` in a condition. A long string is cut at a space and joined with `+`.
- **An import line with an alias** is not counted: it cannot be broken.
- **No tool breaks the lines for us:** `gofmt` does not, and `golines` is not used. A line is broken by the author.
- **An exception** is a `//nolint:lll` with the reason, for a line that cannot be broken (a map entry of two long names, which `gofmt` puts back on one line). It is rare.

Why: a line that fits the screen is read without scrolling and diffs side by side.
