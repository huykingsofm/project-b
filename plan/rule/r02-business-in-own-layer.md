# R2. Business stays in its own layer (hard)

A layer never carries the business of another layer: not in its names, comments, options, queries or tests. An inner layer offers a mechanism that knows nothing of who uses it (`config.List[T]` with the options `Sensitive` and `Each`, a table of channels), and the layer that owns the business declares it, fills it in and interprets it (`auth` declares `auth.token_keys` and checks the keys; `config` has never heard of keys, tokens or sessions).

A word of one layer's business in another layer is a leak, to be fixed in the change that finds it, by [R11](r11-refactor.md). [R1](r01-layer-dependency.md) says who may import whom; [R2](r02-business-in-own-layer.md) says what each layer may know.
