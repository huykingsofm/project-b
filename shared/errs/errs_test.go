package errs_test

import (
	"errors"
	"testing"

	"github.com/huykingsofm/project-b/shared/errs"
)

func TestJoinIsNilForNoErrorAndHoldsEveryOneThatWasReturned(t *testing.T) {
	if err := errs.Join(nil, nil); err != nil {
		t.Fatalf("no error joined to %v", err)
	}
	a, b := errors.New("a"), errors.New("b")
	err := errs.Join(a, nil, b)
	if !errors.Is(err, a) || !errors.Is(err, b) {
		t.Fatalf("%v does not hold both errors", err)
	}
}
