# R17. The plan and the scheduler are not in `shared`

See [R3](r03-shared-code.md). In particular: the plan and the scheduler are part of `sim/action`, and they are game logic. `shared` never takes a package because it is "common to the three engines", and a package of `shared` never imports `sim`, `engine`, `config` or anything else of the project. Being common to several engines does not mean having no business.
