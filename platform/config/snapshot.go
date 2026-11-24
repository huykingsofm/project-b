package config

// Snapshot is the value of every variable at one moment. It never changes after it is built, so
// any number of goroutines can read it, and a tick that reads one snapshot at its start sees one
// version of the configuration.
type Snapshot struct {
	version uint64
	vals    []any
}

// Version identifies the snapshot. Values derived from a snapshot are cached with the version
// they were computed from and recomputed when it changes.
func (s *Snapshot) Version() uint64 { return s.version }

// Defaults returns the snapshot with every variable at its default. The first call, like Build,
// ends the declaration of variables.
func Defaults() *Snapshot {
	s, _, _ := std.build(0, nil)
	return s
}

// Build returns the snapshot of the given version with the overrides applied to the defaults.
func Build(
	version uint64, overrides map[string]string,
) (snap *Snapshot, unknown []string, err error) {
	return std.build(version, overrides)
}
