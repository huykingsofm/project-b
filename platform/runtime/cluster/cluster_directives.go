package cluster

import (
	"github.com/huykingsofm/project-b/platform/runtime/router"
	"github.com/huykingsofm/project-b/platform/sim"
)

// ClusterDirectives is what a cluster needs of the directive router: to take the requests and the
// acknowledgements, and to publish the responses.
type ClusterDirectives interface {
	TakeRequests() []router.PublishedDirectiveRequest
	TakeResponseAcknowledgements() []router.ResponseAcknowledgement
	// PublishResponse puts a response where its engine takes it, and reports false when the engine
	// has left.
	PublishResponse(to sim.StateID, r sim.DirectiveResponse) bool
}

var _ ClusterDirectives = (*router.DirectiveRouter)(nil)
