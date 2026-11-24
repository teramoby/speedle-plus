#!/usr/bin/env python3
"""Render and validate Scoop and winget manifests for spctl from checksums.txt.

Produces:
  - <output-dir>/spctl.json (Scoop manifest)
  - <output-dir>/winget/manifests/t/Teramoby/spctl/<X.Y.Z>/Teramoby.spctl{,.installer,.locale.en-US}.yaml
  - <output-dir>/spctl-winget-manifests.zip (deterministic winget manifest bundle)
"""

import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import sys
import zipfile

VERSION_RE = re.compile(r"^v([0-9]+\.[0-9]+\.[0-9]+)$")
DIGEST_RE = re.compile(r"^[0-9a-f]{64}$")
ASSET_RE = re.compile(r"^[A-Za-z0-9._+-]+$")
PACKAGE_ID = "Teramoby.spctl"
WINGET_SCHEMA_VERSION = "1.6.0"
DOS_MIN_EPOCH = int(datetime(1980, 1, 1, 0, 0, 0, tzinfo=timezone.utc).timestamp())

WINDOWS_ASSETS = {
    "amd64": "spctl_windows_amd64.exe",
    "arm64": "spctl_windows_arm64.exe",
}


class ManifestError(Exception):
    """Raised when checksums, inputs, or rendered manifests are invalid."""


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
        raise ManifestError(f"missing {description}: {path} ({exc})") from exc
    if stat.S_ISLNK(meta.st_mode) or not stat.S_ISREG(meta.st_mode):
        raise ManifestError(f"{description} must be a regular file: {path}")
    if meta.st_size <= 0:
        raise ManifestError(f"{description} must not be empty: {path}")
    return path.resolve()


def load_checksums(path: Path) -> dict[str, str]:
    ensure_regular_nonempty_file(path, "checksums file")
    entries: dict[str, str] = {}
    for line_no, raw in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        if not raw.strip():
            continue
        parts = raw.split()
        if len(parts) != 2 or not DIGEST_RE.fullmatch(parts[0]) or not ASSET_RE.fullmatch(parts[1]):
            raise ManifestError(f"{path}:{line_no}: expected '<64-hex-sha256>  <asset>'")
        digest, asset = parts
        if asset in entries:
            raise ManifestError(f"{path}:{line_no}: duplicate checksum entry for {asset}")
        entries[asset] = digest
    if not entries:
        raise ManifestError(f"{path}: checksums file has no entries")
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


def parse_zip_date_time(raw_epoch: str | None) -> tuple[int, int, int, int, int, int]:
    value = raw_epoch if raw_epoch is not None else os.environ.get("SOURCE_DATE_EPOCH", str(DOS_MIN_EPOCH))
    if not re.fullmatch(r"[0-9]+", value):
        raise ManifestError("SOURCE_DATE_EPOCH (or --source-date-epoch) must be a non-negative integer timestamp")
    clamped = max(int(value), DOS_MIN_EPOCH)
    dt = datetime.fromtimestamp(clamped, tz=timezone.utc)
    return (dt.year, dt.month, dt.day, dt.hour, dt.minute, dt.second)


def render_scoop_manifest(template_path: Path, bare_version: str, sha_amd64: str, sha_arm64: str) -> str:
    ensure_regular_nonempty_file(template_path, "Scoop template")
    rendered = (
        template_path.read_text(encoding="utf-8")
        .replace("{VERSION}", bare_version)
        .replace("{SHA256_AMD64}", sha_amd64)
        .replace("{SHA256_ARM64}", sha_arm64)
    )
    parsed = json.loads(rendered)
    if parsed.get("version") != bare_version:
        raise ManifestError("rendered Scoop manifest version mismatch")
    if parsed.get("license") != "UPL-1.0":
        raise ManifestError("rendered Scoop manifest license must be UPL-1.0")
    for scoop_arch, expected_sha, asset_name in (
        ("64bit", sha_amd64, WINDOWS_ASSETS["amd64"]),
        ("arm64", sha_arm64, WINDOWS_ASSETS["arm64"]),
    ):
        entry = parsed["architecture"][scoop_arch]
        expected_url = f"https://github.com/teramoby/speedle-plus/releases/download/v{bare_version}/{asset_name}#/spctl.exe"
        if entry["url"] != expected_url or entry["hash"] != expected_sha:
            raise ManifestError(f"rendered Scoop manifest {scoop_arch} entry mismatch")
    return json.dumps(parsed, indent=2) + "\n"


def render_winget_files(bare_version: str, sha_amd64: str, sha_arm64: str) -> dict[str, str]:
    version_yaml = "\n".join(
        [
            f"# yaml-language-server: $schema=https://aka.ms/winget-manifest.version.{WINGET_SCHEMA_VERSION}.schema.json",
            "",
            f"PackageIdentifier: {PACKAGE_ID}",
            f"PackageVersion: {bare_version}",
            "DefaultLocale: en-US",
            "ManifestType: version",
            f"ManifestVersion: {WINGET_SCHEMA_VERSION}",
            "",
        ]
    )
    locale_yaml = "\n".join(
        [
            f"# yaml-language-server: $schema=https://aka.ms/winget-manifest.defaultLocale.{WINGET_SCHEMA_VERSION}.schema.json",
            "",
            f"PackageIdentifier: {PACKAGE_ID}",
            f"PackageVersion: {bare_version}",
            "PackageLocale: en-US",
            "Publisher: The Speedle Authors",
            "PublisherUrl: https://engine.speedle.io",
            "PublisherSupportUrl: https://github.com/teramoby/speedle-plus/issues",
            "Author: The Speedle Authors",
            "PackageName: Speedle spctl",
            "PackageUrl: https://engine.speedle.io",
            "License: UPL-1.0",
            "LicenseUrl: https://github.com/teramoby/speedle-plus/blob/master/LICENSE",
            "ShortDescription: Command-line tool for Speedle authorization policies",
            "Moniker: spctl",
            "Tags:",
            "  - authorization",
            "  - cli",
            "  - policy",
            "  - rbac",
            "  - abac",
            "  - speedle",
            f"ReleaseNotesUrl: https://github.com/teramoby/speedle-plus/releases/tag/v{bare_version}",
            "ManifestType: defaultLocale",
            f"ManifestVersion: {WINGET_SCHEMA_VERSION}",
            "",
        ]
    )
    installer_yaml = "\n".join(
        [
            f"# yaml-language-server: $schema=https://aka.ms/winget-manifest.installer.{WINGET_SCHEMA_VERSION}.schema.json",
            "",
            f"PackageIdentifier: {PACKAGE_ID}",
            f"PackageVersion: {bare_version}",
            "InstallerType: portable",
            "Commands:",
            "  - spctl",
            "Installers:",
            "  - Architecture: x64",
            f"    InstallerUrl: https://github.com/teramoby/speedle-plus/releases/download/v{bare_version}/{WINDOWS_ASSETS['amd64']}",
            f"    InstallerSha256: {sha_amd64.upper()}",
            "  - Architecture: arm64",
            f"    InstallerUrl: https://github.com/teramoby/speedle-plus/releases/download/v{bare_version}/{WINDOWS_ASSETS['arm64']}",
            f"    InstallerSha256: {sha_arm64.upper()}",
            "ManifestType: installer",
            f"ManifestVersion: {WINGET_SCHEMA_VERSION}",
            "",
        ]
    )
    rel_prefix = f"manifests/t/Teramoby/spctl/{bare_version}"
    files = {
        f"{rel_prefix}/{PACKAGE_ID}.yaml": version_yaml,
        f"{rel_prefix}/{PACKAGE_ID}.installer.yaml": installer_yaml,
        f"{rel_prefix}/{PACKAGE_ID}.locale.en-US.yaml": locale_yaml,
    }
    for name, body in files.items():
        if "Speedle+" in body:
            raise ManifestError(f"{name}: forbidden brand spelling 'Speedle+'")
    return files


def write_deterministic_zip(
    zip_path: Path,
    files: dict[str, str],
    zip_date_time: tuple[int, int, int, int, int, int],
) -> None:
    if zip_path.exists():
        zip_path.unlink()
    with zipfile.ZipFile(zip_path, mode="w") as archive:
        for arcname in sorted(files):
            info = zipfile.ZipInfo(filename=arcname, date_time=zip_date_time)
            info.compress_type = zipfile.ZIP_DEFLATED
            info.create_system = 3  # Unix
            info.external_attr = (stat.S_IFREG | 0o644) << 16
            archive.writestr(info, files[arcname].encode("utf-8"), compress_type=zipfile.ZIP_DEFLATED, compresslevel=9)


def generate_windows_manifests(
    *,
    version_tag: str,
    checksums_path: Path,
    scoop_template_path: Path,
    output_dir: Path,
    binaries_dir: Path | None = None,
    source_date_epoch: str | None = None,
    update_checksums_file: bool = False,
) -> tuple[Path, Path, list[Path]]:
    match = VERSION_RE.fullmatch(version_tag)
    if match is None:
        raise ManifestError(f"version must look like v1.2.3, got {version_tag!r}")
    bare_version = match.group(1)
    zip_dt = parse_zip_date_time(source_date_epoch)

    checksums = load_checksums(checksums_path)
    digests: dict[str, str] = {}
    for arch_key, asset_name in WINDOWS_ASSETS.items():
        if asset_name not in checksums:
            raise ManifestError(f"{checksums_path}: missing required checksum for {asset_name}")
        expected = checksums[asset_name]
        if binaries_dir is not None:
            binary_path = ensure_regular_nonempty_file(binaries_dir / asset_name, f"Windows binary {asset_name}")
            actual = sha256_file(binary_path)
            if actual != expected:
                raise ManifestError(
                    f"checksum mismatch for {binary_path}: expected {expected} from {checksums_path}, got {actual}"
                )
        digests[arch_key] = expected

    output_dir.mkdir(parents=True, exist_ok=True)

    scoop_text = render_scoop_manifest(scoop_template_path, bare_version, digests["amd64"], digests["arm64"])
    scoop_path = output_dir / "spctl.json"
    scoop_path.write_text(scoop_text, encoding="utf-8")

    winget_files = render_winget_files(bare_version, digests["amd64"], digests["arm64"])
    winget_written: list[Path] = []
    for rel_path, content in sorted(winget_files.items()):
        dest = output_dir / "winget" / rel_path
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_text(content, encoding="utf-8")
        winget_written.append(dest)

    bundle_path = output_dir / "spctl-winget-manifests.zip"
    write_deterministic_zip(bundle_path, winget_files, zip_dt)

    if update_checksums_file:
        updated = dict(checksums)
        updated[scoop_path.name] = sha256_file(scoop_path)
        updated[bundle_path.name] = sha256_file(bundle_path)
        write_checksums(checksums_path, updated)

    return scoop_path, bundle_path, winget_written


def build_arg_parser() -> argparse.ArgumentParser:
    default_scoop_tmpl = Path(__file__).resolve().parent.parent / "scoop" / "spctl.json.tmpl"
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True, help="Release semantic version tag (vX.Y.Z)")
    parser.add_argument("--checksums", required=True, type=Path, help="Path to checksums.txt")
    parser.add_argument("--binaries-dir", type=Path, help="Optional directory to verify spctl_windows_*.exe bytes")
    parser.add_argument("--scoop-template", type=Path, default=default_scoop_tmpl, help="Path to spctl.json.tmpl")
    parser.add_argument("--output-dir", required=True, type=Path, help="Destination directory for rendered outputs")
    parser.add_argument("--source-date-epoch", help="Unix epoch timestamp for reproducible zip metadata")
    parser.add_argument(
        "--update-checksums",
        action="store_true",
        help="Add or replace spctl.json and spctl-winget-manifests.zip SHA-256 digests in --checksums",
    )
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_arg_parser().parse_args(argv)
    try:
        scoop_path, bundle_path, winget_files = generate_windows_manifests(
            version_tag=args.version,
            checksums_path=args.checksums,
            scoop_template_path=args.scoop_template,
            output_dir=args.output_dir,
            binaries_dir=args.binaries_dir,
            source_date_epoch=args.source_date_epoch,
            update_checksums_file=args.update_checksums,
        )
    except ManifestError as exc:
        print(f"update-windows-manifests.py: {exc}", file=sys.stderr)
        return 1
    print(f"wrote {scoop_path.name}, {bundle_path.name} ({len(winget_files)} winget YAML files)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
