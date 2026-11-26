package engine

import (
	"sync/atomic"

	"github.com/huykingsofm/project-b/platform/sim"
)

// ClusterController is what an engine needs of its cluster to change which engines the cluster
// has: it hands over what its actions asked for when a tick is done. An engine in an environment
// with none ignores the requests: a replay has none, since the log says which engine was removed
// and when.
type ClusterController interface {
	// RequestEngineRemoval asks the cluster to remove the engine with the ID. It does not wait.
	RequestEngineRemoval(id sim.StateID)
}

// ClusterControl is the ClusterController of a cluster that runs. It is made before the cluster,
// since the engines are made in an environment that holds it, and NewCluster attaches the cluster
// to it.
type ClusterControl struct{ cluster atomic.Pointer[Cluster] }

// NewClusterControl makes a control that has no cluster yet: a request is dropped until NewCluster
// is given an environment that holds it.
func NewClusterControl() *ClusterControl { return &ClusterControl{} }

var _ ClusterController = (*ClusterControl)(nil)

// RequestEngineRemoval passes the request to the cluster.
func (c *ClusterControl) RequestEngineRemoval(id sim.StateID) {
	if cluster := c.cluster.Load(); cluster != nil {
		cluster.RequestEngineRemoval(id)
	}
}
