#!/usr/bin/env python3
"""Build deterministic .deb and .rpm packages from checksum-verified release binaries.

Never compiles or rebuilds Go binaries: every packaged executable is verified
against checksums.txt before nFPM runs, and the packaged LICENSE file must be a
regular file from the checked-out release source commit.
"""

import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys
import tempfile

VERSION_RE = re.compile(r"^v([0-9]+\.[0-9]+\.[0-9]+)$")
DIGEST_RE = re.compile(r"^[0-9a-f]{64}$")
ASSET_RE = re.compile(r"^[A-Za-z0-9._+-]+$")

PACKAGES = ("spctl", "speedle-pms", "speedle-ads")
ARCHES = (
    ("amd64", "amd64", "x86_64"),
    ("arm64", "arm64", "aarch64"),
)


class PackagingError(Exception):
    """Raised when input validation, checksum verification, or packaging fails."""


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(65536), b""):
            digest.update(chunk)
    return digest.hexdigest()


def ensure_regular_nonempty_file(path: Path, description: str) -> Path:
    try:
        meta = path.lstat()
    except OSError as exc:
        raise PackagingError(f"missing {description}: {path} ({exc})") from exc
    if stat.S_ISLNK(meta.st_mode) or not stat.S_ISREG(meta.st_mode):
        raise PackagingError(f"{description} must be a regular file, not a symlink or directory: {path}")
    if meta.st_size <= 0:
        raise PackagingError(f"{description} must not be empty: {path}")
    return path.resolve()


def load_checksums(path: Path) -> dict[str, str]:
    ensure_regular_nonempty_file(path, "checksums file")
    entries: dict[str, str] = {}
    for line_no, raw in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        if not raw.strip():
            continue
        parts = raw.split()
        if len(parts) != 2 or not DIGEST_RE.fullmatch(parts[0]) or not ASSET_RE.fullmatch(parts[1]):
            raise PackagingError(f"{path}:{line_no}: expected '<64-hex-sha256>  <asset>'")
        digest, asset = parts
        if asset in entries:
            raise PackagingError(f"{path}:{line_no}: duplicate checksum entry for {asset}")
        entries[asset] = digest
    if not entries:
        raise PackagingError(f"{path}: checksums file has no entries")
    return entries


def write_checksums(path: Path, entries: dict[str, str]) -> None:
    lines = [f"{digest}  {asset}\n" for asset, digest in entries.items()]
    tmp = path.parent / f".{path.name}.tmp.{os.getpid()}"
    try:
        tmp.write_text("".join(lines), encoding="utf-8")
        tmp.replace(path)
    finally:
        if tmp.exists():
            tmp.unlink()


def parse_epoch(raw_epoch: str | None) -> tuple[int, str]:
    value = raw_epoch if raw_epoch is not None else os.environ.get("SOURCE_DATE_EPOCH")
    if value is None or not re.fullmatch(r"[0-9]+", value):
        raise PackagingError("SOURCE_DATE_EPOCH (or --source-date-epoch) must be a non-negative integer timestamp")
    epoch = int(value)
    iso_mtime = datetime.fromtimestamp(epoch, tz=timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    return epoch, iso_mtime


def yaml_escape_double_quoted(value: str) -> str:
    return json.dumps(value)[1:-1]


def render_template(template_path: Path, mapping: dict[str, str]) -> str:
    text = template_path.read_text(encoding="utf-8")
    for key, value in mapping.items():
        text = text.replace("{" + key + "}", yaml_escape_double_quoted(value))
    return text


def build_linux_packages(
    *,
    version_tag: str,
    dist_dir: Path,
    checksums_path: Path,
    license_path: Path,
    output_dir: Path,
    linux_root: Path,
    nfpm_bin: str,
    source_date_epoch: int,
    mtime_rfc3339: str,
    update_checksums_file: bool = False,
) -> list[Path]:
    match = VERSION_RE.fullmatch(version_tag)
    if match is None:
        raise PackagingError(f"version must look like v1.2.3, got {version_tag!r}")
    bare_version = match.group(1)

    license_file = ensure_regular_nonempty_file(license_path, "release LICENSE")
    checksums = load_checksums(checksums_path)

    nfpm_dir = linux_root / "nfpm"
    systemd_dir = linux_root / "systemd"
    etc_dir = linux_root / "etc"
    scripts_dir = linux_root / "scripts"
    for required_dir in (nfpm_dir, systemd_dir, etc_dir, scripts_dir):
        if not required_dir.is_dir():
            raise PackagingError(f"missing Linux packaging directory: {required_dir}")

    # Verify every required Linux binary exists as a regular file and matches checksums.txt
    # before invoking nFPM, so a corrupted or mismatched binary never produces an archive.
    verified_binaries: dict[tuple[str, str], Path] = {}
    for pkg in PACKAGES:
        for goarch, _deb_arch, _rpm_arch in ARCHES:
            asset_name = f"{pkg}_linux_{goarch}"
            if asset_name not in checksums:
                raise PackagingError(f"{checksums_path}: missing required checksum for {asset_name}")
            binary_path = ensure_regular_nonempty_file(dist_dir / asset_name, f"release binary {asset_name}")
            actual = sha256_file(binary_path)
            expected = checksums[asset_name]
            if actual != expected:
                raise PackagingError(
                    f"checksum mismatch for {binary_path}: expected {expected} from {checksums_path}, got {actual}"
                )
            verified_binaries[(pkg, goarch)] = binary_path

    output_dir.mkdir(parents=True, exist_ok=True)
    built_paths: list[Path] = []

    env = os.environ.copy()
    env["SOURCE_DATE_EPOCH"] = str(source_date_epoch)
    env["TZ"] = "UTC"

    with tempfile.TemporaryDirectory(prefix="speedle-nfpm-cfg-") as stage_raw:
        stage_dir = Path(stage_raw)
        for pkg in PACKAGES:
            template_path = ensure_regular_nonempty_file(nfpm_dir / f"{pkg}.yaml.tmpl", f"nFPM template {pkg}")
            for goarch, deb_arch, rpm_arch in ARCHES:
                config_text = render_template(
                    template_path,
                    {
                        "ARCH": goarch,
                        "VERSION": bare_version,
                        "MTIME": mtime_rfc3339,
                        "BINARY_SRC": str(verified_binaries[(pkg, goarch)]),
                        "LICENSE_SRC": str(license_file),
                        "SYSTEMD_DIR": str(systemd_dir.resolve()),
                        "ETC_DIR": str(etc_dir.resolve()),
                        "SCRIPTS_DIR": str(scripts_dir.resolve()),
                    },
                )
                config_file = stage_dir / f"{pkg}_{goarch}.yaml"
                config_file.write_text(config_text, encoding="utf-8")

                targets = (
                    ("deb", output_dir / f"{pkg}_{bare_version}-1_{deb_arch}.deb"),
                    ("rpm", output_dir / f"{pkg}-{bare_version}-1.{rpm_arch}.rpm"),
                )
                for packager, target_path in targets:
                    if target_path.exists():
                        target_path.unlink()
                    cmd = [
                        nfpm_bin,
                        "package",
                        "--config",
                        str(config_file),
                        "--packager",
                        packager,
                        "--target",
                        str(target_path),
                    ]
                    completed = subprocess.run(cmd, env=env, text=True, capture_output=True, check=False)
                    if completed.returncode != 0:
                        detail = (completed.stderr or completed.stdout).strip()
                        raise PackagingError(f"nfpm ({packager} {pkg} {goarch}) failed: {detail}")
                    ensure_regular_nonempty_file(target_path, f"built {packager} archive {target_path.name}")
                    built_paths.append(target_path)

    if update_checksums_file:
        updated = dict(checksums)
        for pkg_path in sorted(built_paths, key=lambda p: p.name):
            updated[pkg_path.name] = sha256_file(pkg_path)
        write_checksums(checksums_path, updated)

    return built_paths


def build_arg_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True, help="Release semantic version tag (vX.Y.Z)")
    parser.add_argument("--dist-dir", required=True, type=Path, help="Directory containing *_linux_{amd64,arm64}")
    parser.add_argument("--checksums", required=True, type=Path, help="Path to checksums.txt")
    parser.add_argument("--license", required=True, type=Path, help="Path to LICENSE from the release commit")
    parser.add_argument("--output-dir", type=Path, help="Destination directory for .deb and .rpm archives")
    parser.add_argument("--nfpm-bin", default="nfpm", help="Path or command name for nfpm")
    parser.add_argument("--source-date-epoch", help="Unix epoch timestamp for reproducible archives")
    parser.add_argument(
        "--update-checksums",
        action="store_true",
        help="Add or replace built .deb/.rpm SHA-256 digests in --checksums without duplicates",
    )
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_arg_parser().parse_args(argv)
    output_dir = args.output_dir if args.output_dir is not None else args.dist_dir
    linux_root = Path(__file__).resolve().parent
    try:
        epoch, mtime = parse_epoch(args.source_date_epoch)
        built = build_linux_packages(
            version_tag=args.version,
            dist_dir=args.dist_dir,
            checksums_path=args.checksums,
            license_path=args.license,
            output_dir=output_dir,
            linux_root=linux_root,
            nfpm_bin=args.nfpm_bin,
            source_date_epoch=epoch,
            mtime_rfc3339=mtime,
            update_checksums_file=args.update_checksums,
        )
    except PackagingError as exc:
        print(f"build-packages.py: {exc}", file=sys.stderr)
        return 1
    for path in built:
        print(f"built {path.name}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
