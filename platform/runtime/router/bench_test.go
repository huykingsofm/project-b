package router_test

import (
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/huykingsofm/project-b/platform/runtime/router"
	"github.com/huykingsofm/project-b/platform/runtime/testkit"
	"github.com/huykingsofm/project-b/platform/sim"
)

// BenchmarkDeliver is the send of one message to an engine and the read of it by the engine.
func BenchmarkDeliver(b *testing.B) {
	rt := router.NewRouter()
	in, out := router.NewChannel(1, 1)
	rt.Register(out)
	m := testkit.ToyMessage(1, 1)
	b.ReportAllocs()
	for b.Loop() {
		rt.Deliver(m)
		<-in.In
	}
}

// BenchmarkDeliverFromManyEngines is the send and the read of a message by each of as many
// goroutines as the benchmark has CPUs, each an engine with its own channel in the one router.
func BenchmarkDeliverFromManyEngines(b *testing.B) {
	engines := runtime.GOMAXPROCS(0)
	rt := router.NewRouter()
	ins := make([]router.IncomingChannel, engines)
	for i := range engines {
		in, out := router.NewChannel(sim.StateID(i+1), 1)
		rt.Register(out)
		ins[i] = in
	}
	var next atomic.Uint64
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := int(next.Add(1)-1) % engines
		m := testkit.ToyMessage(sim.StateID(i+1), 1)
		for pb.Next() {
			rt.Deliver(m)
			<-ins[i].In
		}
	})
}

// BenchmarkReturn is the return of a message to a sender that is there, and the read of it.
func BenchmarkReturn(b *testing.B) {
	rt := router.NewRouter()
	in, out := router.NewChannel(1, 1)
	rt.Register(out)
	m := testkit.ToyMessage(2, 1).Stamp(sim.Header{From: 1})
	b.ReportAllocs()
	for b.Loop() {
		rt.Return(m, sim.DeliveryFailureFull)
		<-in.Back
	}
}
