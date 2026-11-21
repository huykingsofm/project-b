package action

import (
	"reflect"
	"testing"

	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

func TestAnAddressAndAHeaderComeBackAsTheyWereWrittenAndCutBytesAreAnError(t *testing.T) {
	h := Header{From: ZoneLayerAddress(300), Seq: 1 << 40}
	var w binarycodec.Writer
	h.Encode(&w)
	r := binarycodec.NewReader(w.B)
	if got := DecodeHeader(r, 1); r.Err() != nil || r.Left() != 0 || !reflect.DeepEqual(got, h) {
		t.Fatalf("got %+v, error %v, %d bytes left; want %+v", got, r.Err(), r.Left(), h)
	}
	for cut := range len(w.B) {
		r := binarycodec.NewReader(w.B[:cut])
		DecodeHeader(r, 1)
		if r.Err() == nil {
			t.Fatalf("a header cut at %d of %d bytes was read", cut, len(w.B))
		}
	}
}
