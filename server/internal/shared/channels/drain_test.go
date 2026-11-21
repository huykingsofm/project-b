package channels_test

import (
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/server/internal/shared/channels"
)

func TestDrainTakesWhatIsThereInOrderAndLeavesTheChannelEmpty(t *testing.T) {
	ch := make(chan int, 4)
	ch <- 1
	ch <- 2
	ch <- 3
	if got := channels.Drain(ch); !slices.Equal(got, []int{1, 2, 3}) {
		t.Fatalf("got %v, want [1 2 3]", got)
	}
	if len(ch) != 0 {
		t.Fatalf("%d left in the channel", len(ch))
	}
	if got := channels.Drain(ch); len(got) != 0 {
		t.Fatalf("an empty channel gave %v", got)
	}
}

func TestDrainTakesOnlyWhatWasThereWhenItWasCalled(t *testing.T) {
	ch := make(chan int, 8)
	for i := range 3 {
		ch <- i
	}
	taken := channels.Drain(ch)
	ch <- 99 // arrives after the call
	if len(taken) != 3 || len(ch) != 1 {
		t.Fatalf("took %d, %d left; want 3 and 1", len(taken), len(ch))
	}
}
