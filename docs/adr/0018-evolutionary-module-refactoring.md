# Refactor modules evolutionarily behind stable CLI boundaries

Structural refactors retain the existing `cmd → internal` boundary and Registry-centered product model. Oversized commands will be split into narrow command coordinators and independently testable internal services; new interfaces are introduced only when a concrete testability or performance boundary requires one.

Rebuilding command and service interfaces wholesale would risk Functional Compatibility and obscure regressions in filesystem effects and exit behavior. Incremental extraction keeps those contracts observable while still allowing responsibilities, tests, comments, and measured performance improvements to move into focused modules.
