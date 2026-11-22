package runner

import (
	"fmt"

	"github.com/huykingsofm/project-b/server/internal/services/game/engine/replay"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
)

// Logs holds the records of each engine, in the order they were recorded.
type Logs map[message.Address][]replay.Record

// ReadLogs reads the files that a replay.FileLog wrote under dir.
func ReadLogs(dir string) (Logs, error) {
	engines, err := replay.Engines(dir)
	if err != nil {
		return nil, err
	}
	logs := Logs{}
	for _, e := range engines {
		records, err := replay.ReadAll(dir, e)
		if err != nil {
			return nil, fmt.Errorf("the log of %v: %w", e, err)
		}
		logs[e] = records
	}
	return logs, nil
}
