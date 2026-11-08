# Policy source golden vectors

`golden-vectors.json` is shared with Speedle Cloud. It is copied verbatim
from `packages/shared/fixtures/policy-source/golden-vectors.json` in the
Cloud repository, where `packages/shared/src/policy-source-contract.test.ts`
generates it from a fixed, public Ed25519 test key.

The file must stay byte-identical in both repositories: the Cloud and Engine
test suites assert the same canonical byte encodings, state hashes, commit
hashes and signatures against it. Do not edit, reformat or regenerate it
here; regenerate it in the Cloud repository and copy it over.

**TEST ONLY.** Never use the key, or any value in this directory, in a real
deployment.
