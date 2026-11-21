# R19. Actions do not fail and never leave their engine

An action returns no error: when what it was asked for is not possible it changes nothing, or says so through the state (a refusal), and that is its result. What must happen outside the engine (a message to another engine, an action for itself at a later tick) goes in the registry, and the engine does it when the plan has run. Every send names its `OnFail` (what a failed delivery means); a send that makes no choice does not compile.
