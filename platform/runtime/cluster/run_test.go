package cluster_test

import (
	"context"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/runtime/cluster"
	"github.com/huykingsofm/project-b/platform/runtime/testkit"
)

func TestTheClusterOfAReplayDoesNotRunAndSaysSoInsteadOfPanicking(t *testing.T) {
	c, err := cluster.NewReplayCluster(testkit.NewToyEnv(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Run(context.Background(), time.Millisecond); err == nil {
		t.Fatal("a cluster of a replay, which has no orchestrator, ran")
	}
}
