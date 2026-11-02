# Archive-stage validation records

> English translation of [README.md](README.md), added 2026-11-02. This preserves the archived 2026-10-29 versions, observations, and time-specific statements; it adds no measurements.

These checks took place during archival on 2026-10-29. They are not rerun performance experiments and do not substitute for original preparation-stage terminal logs.

- `python3 verify_archive.py` passed per-file SHA checks, 107×5 groups, 90 HTTP windows, 13 clean steps, 734 samples, 6 binary hashes, consistency of 15 executed source/script files, Markdown links, and high-confidence credential-pattern checks. The verifier itself is in the manifest; Git commits cover MANIFEST.
- `/opt/homebrew/bin/go vet ./...` in this directory: exit 0, no output; pinned Go1.27.2 and original dependencies, no new engine experiment.
- `bash -n run-all.sh stress.sh build-linux.sh`：exit 0。
- Parent-module isolation: see [parent-module-isolation.txt](parent-module-isolation.txt). Ordinary repository-wide go list found two existing GOPATH sample import errors, which were not modified. Listing with -e confirmed the nested archive module is excluded from parent-module ./....
- Both PNGs were opened and viewed in the archival session; SVGs are also retained. No additional performance data were derived from screenshots.
- The repository is public. Archive account paths/authentication metadata were redacted; original/public hashes and transformations were retained separately. Pattern checks validate archive integrity and do not constitute a complete security audit.
