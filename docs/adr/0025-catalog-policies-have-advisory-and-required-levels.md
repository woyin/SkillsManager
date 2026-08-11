# Catalog Policies have advisory and required levels

Team Catalog Policies are classified as advisory or required. Both appear in a Curation Plan, while only an unsatisfied required policy makes `sm plan --check` return a nonzero status for CI; interactive application remains explicitly confirmed.

A single blocking level would make early governance adoption needlessly disruptive, and advisory-only rules could not protect a team's actual baseline. Two explicit levels allow teams to turn proven practices into enforceable controls without conflating guidance with policy.
