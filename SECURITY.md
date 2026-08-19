# Security

This repository is a destructive runtime experiment, not a security boundary.

`GoroutineAbortHard` deliberately skips user defers and can strand locks,
transactions, protocols, files, memory ownership conventions, and arbitrary
application invariants. It must not be used to execute untrusted code or to
replace process, Wasm, container, VM, or capability isolation.

Run only inside the bounded container workflow. Do not install the patched
toolchain over a host Go installation. Do not apply only part of the patch
series.

Please report runtime-integrity defects, wrong-target termination, container
escape, or evidence-integrity problems through a private security channel once
one is configured for the repository. Ordinary semantic limitations and
reproduction failures can use public issues after publication.
