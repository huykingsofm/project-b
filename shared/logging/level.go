package logging

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// Level is how serious a log line is.
type Level uint8

// String is the text written after `level=`.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	}
	return "unknown"
}
