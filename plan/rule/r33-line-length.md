# R33. A line is at most 100 columns (hard)

- **The limit:** no line of Go code or of a comment is longer than 100 columns, a tab counting as 4. `lll` checks it (`make lint`, set in `.golangci.yml`), and `make ci` runs it, so a longer line fails the commit.
- **Comments are wrapped** at the limit, a paragraph to a block of lines; a list item keeps its hanging indent.
- **Code is broken at its structure:** after a comma inside the brackets of a call, a signature or a literal (one line for the arguments, or one for each when they do not fit), or after `&&` and `||` in a condition. A long string is cut at a space and joined with `+`.
- **An import line with an alias** is not counted: it cannot be broken.
- **No tool breaks the lines for us:** `gofmt` does not, and `golines` is not used. A line is broken by the author.
- **An exception** is a `//nolint:lll` with the reason, for a line that cannot be broken (a map entry of two long names, which `gofmt` puts back on one line). It is rare.

Why: a line that fits the screen is read without scrolling and diffs side by side.
