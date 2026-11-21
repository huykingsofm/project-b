// Package config holds the numbers and settings of the game: variables that the code declares
// with a type, a default and options, an immutable Snapshot of the value of every variable, and
// a Store that swaps the snapshot as a whole.
//
// A package that owns a setting declares it with NewVar; this package knows nothing of what the
// settings mean and nothing of the kinds of value they hold. A variable may be of any type that
// encoding/json can read, and a new kind needs no change here: the declaration carries its own
// parsing and its own checks. A value comes in as the text of its JSON, as it is stored, and is
// parsed into the exact type of the variable, so a value of the wrong type or outside its limits
// is refused whole. An engine takes the snapshot once at the start of its tick and passes it
// through its context, so a whole tick sees one version.
package config
