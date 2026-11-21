# R9. In memory first (pattern)

A repository that is read often has an in-memory layer, an adapter that implements the same port as the database one, and that layer is the source of truth: it is loaded when the process starts and the database is written afterwards, by a bounded queue that keeps the order of the writes and is written out at shutdown. A write that depends on the database (a unique name, a foreign key) stays synchronous. The pattern assumes that one process writes a row; a process split brings an owner fence.
