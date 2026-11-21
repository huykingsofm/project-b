# R14. The protocol is not frozen until the first release

Until the first release the protocol (`proto/`) is not frozen and `buf breaking` is not run. A breaking change is made whenever it makes the design simpler or sounder; there are no compatibility shims and no "additive only" workarounds. At release the owner decides when compatibility checking starts.
