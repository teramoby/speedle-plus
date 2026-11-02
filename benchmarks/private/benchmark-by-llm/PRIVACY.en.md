# Public scope, redaction, and integrity

> English translation of [PRIVACY.md](PRIVACY.md), added 2026-11-02. This preserves the archived 2026-10-29 versions, observations, and time-specific statements; it adds no measurements.

The destination repository is public; private is only a directory name. After measurement, the user explicitly requested publication of this archive. The public copy follows a minimum-necessary scope.

- Local account directory components under `/Users/` and `/home/` were replaced with LOCAL_USER / REMOTE_USER. Host aliases, CPU model, times, PIDs, performance figures, and CI/paging counts were retained for environment auditing.
- Authentication-related permissions fields were removed from GitHub API snapshots. Repository/contributor identifiers needed by the queries, contribution counts, and statistics were retained. Irrelevant avatar/follow links, issue/release bodies, and uploader profiles were removed. No API keys, tokens, SSH private keys, authentication configuration, or user-directory contents are included.
- Source snapshots were repackaged with tar uid/gid set to 0 and empty uname/gname fields. macOS xattr/PAX account metadata was removed. Member text received the same directory redaction.
- PNG/SVG figures and source files without the above content retain their original bytes. Measurements, request decisions, rounds, module versions, and original binary hashes were not changed.
- Large executables and the complete package were not committed. Their original SHA-256 values are retained in provenance/build-artifacts.json; they can be rebuilt from the pinned source and dependencies.

[publication-source-records.json](provenance/publication-source-records.json) records original/public hashes and transformations for copied files. [archive-entry-hashes.json](provenance/archive-entry-hashes.json) covers members of the two compressed source/historical archives. [MANIFEST.json](MANIFEST.json) covers the final public files; Git commits cover the manifest itself.

This directory does not claim that public copies are byte-identical to unredacted originals. Go runtime source, pinned dependencies, and both formal execution scripts were additionally checked against the pre-run source snapshot. Post-processing scripts and the report were refined after measurement; those corrections are documented separately.
