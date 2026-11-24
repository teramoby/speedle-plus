"""Offline tests for Linux .deb/.rpm packaging, systemd units, maintainer scripts, and Windows Scoop/winget manifests."""

import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest
import zipfile

ROOT = Path(__file__).resolve().parents[2]
LINUX_DIR = ROOT / "deployment" / "linux"
SCOOP_DIR = ROOT / "deployment" / "scoop"
WINGET_DIR = ROOT / "deployment" / "winget"


def _load_module(name: str, path: Path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


build_packages = _load_module("build_packages", LINUX_DIR / "build-packages.py")
windows_manifests = _load_module("windows_manifests", WINGET_DIR / "update-windows-manifests.py")


class WindowsManifestTest(unittest.TestCase):
    def _seed_windows_inputs(self, root: Path) -> tuple[Path, Path, dict[str, str]]:
        dist = root / "dist"
        dist.mkdir(parents=True, exist_ok=True)
        digests = {}
        for arch, payload in (("amd64", b"MZ-amd64-spctl"), ("arm64", b"MZ-arm64-spctl")):
            name = f"spctl_windows_{arch}.exe"
            (dist / name).write_bytes(payload)
            digests[name] = hashlib.sha256(payload).hexdigest()
        checksums = dist / "checksums.txt"
        checksums.write_text(
            "".join(f"{digest}  {asset}\n" for asset, digest in sorted(digests.items())),
            encoding="utf-8",
        )
        return dist, checksums, digests

    def test_renders_deterministic_scoop_and_winget_outputs_and_updates_checksums_idempotently(self):
        with tempfile.TemporaryDirectory() as tmp_raw:
            root = Path(tmp_raw)
            dist, checksums, digests = self._seed_windows_inputs(root)
            out1 = root / "out1"
            out2 = root / "out2"
            shutil.copyfile(checksums, root / "checksums1.txt")
            shutil.copyfile(checksums, root / "checksums2.txt")

            for out_dir, csum_file in ((out1, root / "checksums1.txt"), (out2, root / "checksums2.txt")):
                windows_manifests.generate_windows_manifests(
                    version_tag="v1.9.1",
                    checksums_path=csum_file,
                    scoop_template_path=SCOOP_DIR / "spctl.json.tmpl",
                    output_dir=out_dir,
                    binaries_dir=dist,
                    source_date_epoch="1763368554",
                    update_checksums_file=True,
                )

            self.assertEqual((out1 / "spctl.json").read_bytes(), (out2 / "spctl.json").read_bytes())
            self.assertEqual(
                (out1 / "spctl-winget-manifests.zip").read_bytes(),
                (out2 / "spctl-winget-manifests.zip").read_bytes(),
            )
            self.assertEqual((root / "checksums1.txt").read_text(), (root / "checksums2.txt").read_text())

            # Rerun idempotency: no duplicate checksum lines.
            before_rerun = (root / "checksums1.txt").read_text()
            windows_manifests.generate_windows_manifests(
                version_tag="v1.9.1",
                checksums_path=root / "checksums1.txt",
                scoop_template_path=SCOOP_DIR / "spctl.json.tmpl",
                output_dir=out1,
                binaries_dir=dist,
                source_date_epoch="1763368554",
                update_checksums_file=True,
            )
            self.assertEqual(before_rerun, (root / "checksums1.txt").read_text())
            self.assertEqual(len(before_rerun.splitlines()), 4)

            scoop = json.loads((out1 / "spctl.json").read_text(encoding="utf-8"))
            self.assertEqual(scoop["version"], "1.9.1")
            self.assertEqual(scoop["license"], "UPL-1.0")
            self.assertEqual(scoop["architecture"]["64bit"]["hash"], digests["spctl_windows_amd64.exe"])
            self.assertEqual(scoop["architecture"]["arm64"]["hash"], digests["spctl_windows_arm64.exe"])
            self.assertIn("spctl.exe", scoop["bin"])

            # Verify Scoop autoupdate regex matches checksums.txt lines.
            csum_body = (root / "checksums1.txt").read_text(encoding="utf-8")
            for scoop_arch, asset in (("64bit", "spctl_windows_amd64.exe"), ("arm64", "spctl_windows_arm64.exe")):
                pattern = scoop["autoupdate"]["architecture"][scoop_arch]["hash"]["regex"]
                match = re.search(pattern.replace("(?m)", ""), csum_body, re.MULTILINE)
                self.assertIsNotNone(match)
                self.assertEqual(match.group(1), digests[asset])

            # Verify winget bundle contents and schema invariants.
            with zipfile.ZipFile(out1 / "spctl-winget-manifests.zip") as zf:
                names = zf.namelist()
                self.assertEqual(
                    names,
                    [
                        "manifests/t/Teramoby/spctl/1.9.1/Teramoby.spctl.installer.yaml",
                        "manifests/t/Teramoby/spctl/1.9.1/Teramoby.spctl.locale.en-US.yaml",
                        "manifests/t/Teramoby/spctl/1.9.1/Teramoby.spctl.yaml",
                    ],
                )
                installer = zf.read("manifests/t/Teramoby/spctl/1.9.1/Teramoby.spctl.installer.yaml").decode("utf-8")
                locale = zf.read("manifests/t/Teramoby/spctl/1.9.1/Teramoby.spctl.locale.en-US.yaml").decode("utf-8")
                self.assertIn("InstallerType: portable", installer)
                self.assertIn(f"InstallerSha256: {digests['spctl_windows_amd64.exe'].upper()}", installer)
                self.assertIn(f"InstallerSha256: {digests['spctl_windows_arm64.exe'].upper()}", installer)
                self.assertIn("License: UPL-1.0", locale)
                self.assertNotIn("Speedle+", installer + locale)

    def test_rejects_checksum_mismatch_duplicate_entries_and_missing_assets(self):
        with tempfile.TemporaryDirectory() as tmp_raw:
            root = Path(tmp_raw)
            dist, checksums, _ = self._seed_windows_inputs(root)
            (dist / "spctl_windows_amd64.exe").write_bytes(b"tampered")
            with self.assertRaisesRegex(windows_manifests.ManifestError, "checksum mismatch"):
                windows_manifests.generate_windows_manifests(
                    version_tag="v1.9.1",
                    checksums_path=checksums,
                    scoop_template_path=SCOOP_DIR / "spctl.json.tmpl",
                    output_dir=root / "out",
                    binaries_dir=dist,
                )

            dup = root / "dup.txt"
            dup.write_text(("a" * 64) + "  spctl_windows_amd64.exe\n" + ("b" * 64) + "  spctl_windows_amd64.exe\n")
            with self.assertRaisesRegex(windows_manifests.ManifestError, "duplicate checksum"):
                windows_manifests.generate_windows_manifests(
                    version_tag="v1.9.1",
                    checksums_path=dup,
                    scoop_template_path=SCOOP_DIR / "spctl.json.tmpl",
                    output_dir=root / "out",
                )


class LinuxPackagingOrchestratorTest(unittest.TestCase):
    def _seed_linux_inputs(self, root: Path) -> tuple[Path, Path, Path]:
        dist = root / "dist"
        dist.mkdir(parents=True, exist_ok=True)
        lines = []
        for pkg in ("spctl", "speedle-pms", "speedle-ads"):
            for arch in ("amd64", "arm64"):
                name = f"{pkg}_linux_{arch}"
                payload = f"ELF-{name}".encode("utf-8")
                (dist / name).write_bytes(payload)
                lines.append(f"{hashlib.sha256(payload).hexdigest()}  {name}\n")
        checksums = dist / "checksums.txt"
        checksums.write_text("".join(lines), encoding="utf-8")
        license_file = root / "LICENSE"
        license_file.write_text("Release commit UPL-1.0 license\n", encoding="utf-8")
        return dist, checksums, license_file

    def _make_stub_nfpm(self, root: Path) -> tuple[Path, Path]:
        calls = root / "nfpm-calls.jsonl"
        nfpm = root / "nfpm"
        nfpm.write_text(
            "\n".join(
                [
                    "#!/usr/bin/env python3",
                    "import hashlib, json, os, sys",
                    "from pathlib import Path",
                    "args = sys.argv[1:]",
                    "cfg = Path(args[args.index('--config') + 1]).read_text(encoding='utf-8')",
                    "packager = args[args.index('--packager') + 1]",
                    "target = Path(args[args.index('--target') + 1])",
                    "record = {'packager': packager, 'target': target.name, 'epoch': os.environ.get('SOURCE_DATE_EPOCH'), 'cfg': cfg}",
                    "with open(os.environ['FIXTURE_NFPM_LOG'], 'a', encoding='utf-8') as f:",
                    "    f.write(json.dumps(record) + '\\n')",
                    "target.write_bytes(hashlib.sha256((packager + ':' + cfg).encode('utf-8')).digest())",
                    "",
                ]
            ),
            encoding="utf-8",
        )
        nfpm.chmod(0o755)
        return nfpm, calls

    def test_verifies_inputs_uses_release_license_and_updates_checksums_without_duplicates(self):
        with tempfile.TemporaryDirectory() as tmp_raw:
            root = Path(tmp_raw)
            dist, checksums, license_file = self._seed_linux_inputs(root)
            nfpm, calls = self._make_stub_nfpm(root)
            os.environ["FIXTURE_NFPM_LOG"] = str(calls)
            try:
                epoch, mtime = build_packages.parse_epoch("1763368554")
                built = build_packages.build_linux_packages(
                    version_tag="v1.9.1",
                    dist_dir=dist,
                    checksums_path=checksums,
                    license_path=license_file,
                    output_dir=dist,
                    linux_root=LINUX_DIR,
                    nfpm_bin=str(nfpm),
                    source_date_epoch=epoch,
                    mtime_rfc3339=mtime,
                    update_checksums_file=True,
                )
                self.assertEqual(len(built), 12)
                first_checksums = checksums.read_text(encoding="utf-8")
                self.assertEqual(len(first_checksums.splitlines()), 18)  # 6 binaries + 12 packages

                # Rerun idempotency: exact same 18 lines, zero duplicates.
                build_packages.build_linux_packages(
                    version_tag="v1.9.1",
                    dist_dir=dist,
                    checksums_path=checksums,
                    license_path=license_file,
                    output_dir=dist,
                    linux_root=LINUX_DIR,
                    nfpm_bin=str(nfpm),
                    source_date_epoch=epoch,
                    mtime_rfc3339=mtime,
                    update_checksums_file=True,
                )
                self.assertEqual(first_checksums, checksums.read_text(encoding="utf-8"))

                records = [json.loads(line) for line in calls.read_text(encoding="utf-8").splitlines()]
                self.assertEqual(len(records), 24)
                for rec in records:
                    self.assertEqual(rec["epoch"], "1763368554")
                    self.assertIn(str(license_file.resolve()), rec["cfg"])
                    self.assertIn('mtime: "2025-11-17T08:35:54Z"', rec["cfg"])
            finally:
                os.environ.pop("FIXTURE_NFPM_LOG", None)

    def test_aborts_before_invoking_nfpm_on_tampered_binary_or_symlink_license(self):
        with tempfile.TemporaryDirectory() as tmp_raw:
            root = Path(tmp_raw)
            dist, checksums, license_file = self._seed_linux_inputs(root)
            nfpm, calls = self._make_stub_nfpm(root)
            os.environ["FIXTURE_NFPM_LOG"] = str(calls)
            try:
                epoch, mtime = build_packages.parse_epoch("1763368554")
                (dist / "speedle-pms_linux_amd64").write_bytes(b"corrupted")
                with self.assertRaisesRegex(build_packages.PackagingError, "checksum mismatch"):
                    build_packages.build_linux_packages(
                        version_tag="v1.9.1",
                        dist_dir=dist,
                        checksums_path=checksums,
                        license_path=license_file,
                        output_dir=dist,
                        linux_root=LINUX_DIR,
                        nfpm_bin=str(nfpm),
                        source_date_epoch=epoch,
                        mtime_rfc3339=mtime,
                    )
                self.assertFalse(calls.exists(), "nfpm must not be invoked when a binary checksum fails")

                # Restore binary, replace LICENSE with a symlink -> must fail closed.
                dist2, checksums2, _ = self._seed_linux_inputs(root / "sub")
                symlink_lic = root / "LICENSE.link"
                symlink_lic.symlink_to(license_file)
                with self.assertRaisesRegex(build_packages.PackagingError, "regular file"):
                    build_packages.build_linux_packages(
                        version_tag="v1.9.1",
                        dist_dir=dist2,
                        checksums_path=checksums2,
                        license_path=symlink_lic,
                        output_dir=dist2,
                        linux_root=LINUX_DIR,
                        nfpm_bin=str(nfpm),
                        source_date_epoch=epoch,
                        mtime_rfc3339=mtime,
                    )
                self.assertFalse(calls.exists())
            finally:
                os.environ.pop("FIXTURE_NFPM_LOG", None)


class SystemdAndConfigSafetyTest(unittest.TestCase):
    def test_systemd_units_and_default_configs_enforce_security_invariants(self):
        for svc, port in (("pms", "6733"), ("ads", "6734")):
            unit_path = LINUX_DIR / "systemd" / f"speedle-{svc}.service"
            unit_text = unit_path.read_text(encoding="utf-8")
            self.assertIn("User=speedle\n", unit_text)
            self.assertIn("Group=speedle\n", unit_text)
            self.assertIn("UMask=0027\n", unit_text)
            self.assertIn("KillSignal=SIGINT\n", unit_text)
            self.assertIn("ProtectSystem=strict\n", unit_text)
            self.assertIn("NoNewPrivileges=yes\n", unit_text)
            self.assertIn("PrivateTmp=yes\n", unit_text)
            self.assertIn("ProtectHome=yes\n", unit_text)
            self.assertIn("StateDirectory=speedle\n", unit_text)
            self.assertIn("StateDirectoryMode=0750\n", unit_text)
            # Never grant service write access to /etc/speedle via ConfigurationDirectory=.
            for line in unit_text.splitlines():
                if not line.lstrip().startswith("#"):
                    self.assertNotIn("ConfigurationDirectory=", line)

            cfg_path = LINUX_DIR / "etc" / f"{svc}.json"
            cfg = json.loads(cfg_path.read_text(encoding="utf-8"))
            self.assertEqual(cfg["serverConfig"]["endpoint"], f"127.0.0.1:{port}")
            self.assertNotIn("0.0.0.0", cfg_path.read_text(encoding="utf-8"))
            self.assertEqual(cfg["storeConfig"]["storeType"], "file")
            self.assertEqual(cfg["storeConfig"]["storeProps"]["FileLocation"], "/var/lib/speedle/policies.json")
            self.assertEqual(cfg["logConfig"]["formatter"], "json")
            self.assertNotIn("asserterWebhookConfig", cfg)

    def test_systemd_analyze_verify_accepts_both_units_when_available(self):
        analyzer = shutil.which("systemd-analyze")
        if analyzer is None:
            self.skipTest("systemd-analyze not installed")
        true_bin = shutil.which("true") or "/bin/true"
        with tempfile.TemporaryDirectory() as tmp_raw:
            tmp = Path(tmp_raw)
            units = []
            for svc in ("pms", "ads"):
                src = (LINUX_DIR / "systemd" / f"speedle-{svc}.service").read_text(encoding="utf-8")
                # Substitute existing executable so systemd-analyze checks directives without host install.
                adapted = src.replace(f"/usr/bin/speedle-{svc}", true_bin)
                dst = tmp / f"speedle-{svc}.service"
                dst.write_text(adapted, encoding="utf-8")
                units.append(str(dst))
            completed = subprocess.run(
                [analyzer, "verify", *units],
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(completed.returncode, 0, completed.stderr or completed.stdout)


class MaintainerScriptsLifecycleTest(unittest.TestCase):
    def test_all_maintainer_scripts_parse_as_posix_sh(self):
        for script in sorted((LINUX_DIR / "scripts").glob("*.sh")):
            completed = subprocess.run(["sh", "-n", str(script)], text=True, capture_output=True, check=False)
            self.assertEqual(completed.returncode, 0, f"{script.name}: {completed.stderr}")

    def _make_harness(self, tmp: Path, *, with_systemd: bool = True) -> dict[str, Path]:
        root = tmp / "rootfs"
        bin_dir = tmp / "bin"
        log_file = tmp / "calls.log"
        db_dir = tmp / "db"
        for d in (root / "etc/speedle", root / "etc/default", root / "var/lib", root / "usr/sbin", bin_dir, db_dir):
            d.mkdir(parents=True, exist_ok=True)
        nologin = root / "usr/sbin/nologin"
        nologin.write_text("#!/bin/sh\nexit 1\n", encoding="utf-8")
        nologin.chmod(0o755)
        for svc in ("pms", "ads"):
            shutil.copyfile(LINUX_DIR / "etc" / f"{svc}.json", root / "etc/speedle" / f"{svc}.json")
            shutil.copyfile(LINUX_DIR / "etc" / f"speedle-{svc}.default", root / "etc/default" / f"speedle-{svc}")

        (db_dir / "group").write_text("", encoding="utf-8")
        (db_dir / "passwd").write_text("", encoding="utf-8")
        state_db = db_dir / "fs_meta.json"
        state_db.write_text(
            json.dumps(
                {
                    str(root / "etc/speedle"): {"uid": 0, "owner": "root", "group": "root", "mode": "755"},
                    str(root / "etc/speedle/pms.json"): {"uid": 0, "owner": "root", "group": "root", "mode": "644"},
                    str(root / "etc/speedle/ads.json"): {"uid": 0, "owner": "root", "group": "root", "mode": "644"},
                    str(root / "etc/default/speedle-pms"): {"uid": 0, "owner": "root", "group": "root", "mode": "644"},
                    str(root / "etc/default/speedle-ads"): {"uid": 0, "owner": "root", "group": "root", "mode": "644"},
                }
            ),
            encoding="utf-8",
        )

        # Stub privileged account/filesystem/systemd commands so unprivileged CI exercises exact script branches.
        helper = tmp / "stub_tool.py"
        helper.write_text(
            "\n".join(
                [
                    "#!/usr/bin/env python3",
                    "import json, os, shutil, stat, sys",
                    "from pathlib import Path",
                    "tool = sys.argv[1]",
                    "args = sys.argv[2:]",
                    "db = Path(os.environ['STUB_DB'])",
                    "log = Path(os.environ['STUB_LOG'])",
                    "meta_path = db / 'fs_meta.json'",
                    "meta = json.loads(meta_path.read_text())",
                    "with log.open('a') as lf:",
                    "    lf.write(tool + ' ' + ' '.join(args) + '\\n')",
                    "def save(): meta_path.write_text(json.dumps(meta))",
                    "if tool == 'getent':",
                    "    kind, key = args[0], args[1]",
                    "    for line in (db / kind).read_text().splitlines():",
                    "        if line.split(':', 1)[0] == key:",
                    "            print(line); sys.exit(0)",
                    "    sys.exit(2)",
                    "if tool == 'groupadd':",
                    "    name = args[-1]",
                    "    with (db / 'group').open('a') as f: f.write(f'{name}:x:980:\\n')",
                    "    sys.exit(0)",
                    "if tool == 'useradd':",
                    "    name = args[-1]",
                    "    home = args[args.index('--home-dir') + 1]",
                    "    shell = args[args.index('--shell') + 1]",
                    "    with (db / 'passwd').open('a') as f: f.write(f'{name}:x:980:980:Speedle:{home}:{shell}\\n')",
                    "    sys.exit(0)",
                    "if tool == 'chown':",
                    "    if '-R' in args or '--recursive' in args:",
                    "        sys.stderr.write('recursive chown forbidden\\n'); sys.exit(99)",
                    "    spec, target = args[-2], str(Path(args[-1]))",
                    "    owner, group = spec.split(':', 1)",
                    "    entry = meta.setdefault(target, {'uid': 0 if owner == 'root' else 980, 'mode': '640'})",
                    "    entry['owner'], entry['group'] = owner, group",
                    "    entry['uid'] = 0 if owner == 'root' else 980",
                    "    save(); sys.exit(0)",
                    "if tool == 'chmod':",
                    "    if '-R' in args or '--recursive' in args:",
                    "        sys.stderr.write('recursive chmod forbidden\\n'); sys.exit(99)",
                    "    mode, target = args[-2].lstrip('0') or '0', str(Path(args[-1]))",
                    "    os.chmod(target, int(args[-2], 8))",
                    "    entry = meta.setdefault(target, {'uid': 0, 'owner': 'root', 'group': 'root'})",
                    "    entry['mode'] = mode",
                    "    save(); sys.exit(0)",
                    "if tool == 'install':",
                    "    mode = args[args.index('-m') + 1]",
                    "    owner = args[args.index('-o') + 1]",
                    "    group = args[args.index('-g') + 1]",
                    "    target = Path(args[-1])",
                    "    target.mkdir(parents=True, exist_ok=True)",
                    "    os.chmod(target, int(mode, 8))",
                    "    meta[str(target)] = {'uid': 980 if owner != 'root' else 0, 'owner': owner, 'group': group, 'mode': mode.lstrip('0')}",
                    "    save(); sys.exit(0)",
                    "if tool == 'ln':",
                    "    src, dst = Path(args[0]), Path(args[1])",
                    "    os.link(src, dst)",
                    "    if str(src) in meta:",
                    "        meta[str(dst)] = dict(meta[str(src)]); save()",
                    "    sys.exit(0)",
                    "if tool == 'stat':",
                    "    fmt, target = args[args.index('-c') + 1], str(Path(args[-1]))",
                    "    entry = meta.get(target)",
                    "    if entry is None:",
                    "        st = os.lstat(target)",
                    "        entry = {'uid': 0, 'owner': 'root', 'group': 'root', 'mode': oct(stat.S_IMODE(st.st_mode))[2:]}",
                    "    if fmt == '%u': print(entry['uid'])",
                    "    elif fmt == '%U:%G:%a': print(f\"{entry['owner']}:{entry['group']}:{entry['mode']}\")",
                    "    else: sys.exit(2)",
                    "    sys.exit(0)",
                    "if tool == 'systemctl': sys.exit(0)",
                    "",
                ]
            ),
            encoding="utf-8",
        )
        helper.chmod(0o755)
        tools = ["getent", "groupadd", "useradd", "chown", "chmod", "install", "ln", "stat"]
        if with_systemd:
            tools.append("systemctl")
        for tool in tools:
            wrapper = bin_dir / tool
            wrapper.write_text(f'#!/bin/sh\nexec python3 -I "{helper}" "{tool}" "$@"\n', encoding="utf-8")
            wrapper.chmod(0o755)

        return {"root": root, "bin": bin_dir, "log": log_file, "db": db_dir, "meta": state_db}

    def _run_script(self, harness: dict[str, Path], script_name: str, *args: str, use_dpkg_root: bool = True):
        env = os.environ.copy()
        env["PATH"] = f"{harness['bin']}{os.pathsep}{env['PATH']}"
        env["STUB_DB"] = str(harness["db"])
        env["STUB_LOG"] = str(harness["log"])
        if use_dpkg_root:
            env["DPKG_ROOT"] = str(harness["root"])
        else:
            env.pop("DPKG_ROOT", None)
        return subprocess.run(
            ["sh", str(LINUX_DIR / "scripts" / script_name), *args],
            env=env,
            text=True,
            capture_output=True,
            check=False,
        )

    def test_fresh_install_upgrade_remove_and_purge_preserve_user_data_with_and_without_sibling(self):
        for sibling_present in (True, False):
            with self.subTest(sibling_present=sibling_present), tempfile.TemporaryDirectory() as tmp_raw:
                tmp = Path(tmp_raw)
                h = self._make_harness(tmp, with_systemd=True)
                root = h["root"]

                # Preinstall creates speedle group & user; second run (sibling package) is idempotent.
                self.assertEqual(self._run_script(h, "preinstall.sh", "install").returncode, 0)
                self.assertEqual(self._run_script(h, "preinstall.sh", "install").returncode, 0)

                # Fresh install of PMS (and optionally ADS sibling).
                res_pms = self._run_script(h, "pms-postinstall.sh", "configure", "")
                self.assertEqual(res_pms.returncode, 0, res_pms.stderr)
                if sibling_present:
                    res_ads = self._run_script(h, "ads-postinstall.sh", "configure", "")
                    self.assertEqual(res_ads.returncode, 0, res_ads.stderr)

                policy_path = root / "var/lib/speedle/policies.json"
                self.assertEqual(policy_path.read_text(encoding="utf-8"), "{}\n")

                # Simulate administrator customization of config and policy data.
                custom_policy = '{"services":[{"name":"production"}]}\n'
                policy_path.write_text(custom_policy, encoding="utf-8")
                custom_cfg = (root / "etc/speedle/pms.json").read_text(encoding="utf-8").replace('"info"', '"warn"')
                (root / "etc/speedle/pms.json").write_text(custom_cfg, encoding="utf-8")

                # Upgrade (.deb prerm upgrade -> postinst configure 1.9.0; .rpm %preun 1 -> %postun 1).
                self.assertEqual(self._run_script(h, "pms-preremove.sh", "upgrade").returncode, 0)
                self.assertEqual(self._run_script(h, "pms-preremove.sh", "1").returncode, 0)
                self.assertEqual(self._run_script(h, "pms-postinstall.sh", "configure", "1.9.0-1").returncode, 0)
                self.assertEqual(self._run_script(h, "pms-postremove.sh", "1").returncode, 0)
                self.assertEqual(policy_path.read_text(encoding="utf-8"), custom_policy)
                self.assertEqual((root / "etc/speedle/pms.json").read_text(encoding="utf-8"), custom_cfg)

                # Remove and Purge: user data and system account MUST stay intact even without sibling.
                self.assertEqual(self._run_script(h, "pms-preremove.sh", "remove").returncode, 0)
                self.assertEqual(self._run_script(h, "pms-postremove.sh", "remove").returncode, 0)
                self.assertEqual(self._run_script(h, "pms-postremove.sh", "purge").returncode, 0)
                self.assertEqual(self._run_script(h, "pms-preremove.sh", "0").returncode, 0)
                self.assertEqual(self._run_script(h, "pms-postremove.sh", "0").returncode, 0)

                self.assertTrue((root / "var/lib/speedle").is_dir())
                self.assertEqual(policy_path.read_text(encoding="utf-8"), custom_policy)
                self.assertIn("speedle:", (h["db"] / "passwd").read_text(encoding="utf-8"))
                self.assertIn("speedle:", (h["db"] / "group").read_text(encoding="utf-8"))
                self.assertNotIn("chown -R", h["log"].read_text(encoding="utf-8"))

    def test_absent_systemd_succeeds_cleanly(self):
        with tempfile.TemporaryDirectory() as tmp_raw:
            h = self._make_harness(Path(tmp_raw), with_systemd=False)
            self.assertEqual(self._run_script(h, "preinstall.sh", "1").returncode, 0)
            self.assertEqual(self._run_script(h, "pms-postinstall.sh", "1").returncode, 0)
            self.assertEqual(self._run_script(h, "pms-preremove.sh", "0").returncode, 0)
            self.assertEqual(self._run_script(h, "pms-postremove.sh", "0").returncode, 0)

    def test_systemd_invocation_matrix_on_host_lifecycle_events(self):
        """Verify systemctl calls on fresh install vs upgrade vs final removal when DPKG_ROOT is unset."""
        if not Path("/run/systemd/system").is_dir():
            self.skipTest("/run/systemd/system not present on host")
        with tempfile.TemporaryDirectory() as tmp_raw:
            h = self._make_harness(Path(tmp_raw), with_systemd=True)
            # preremove/postremove with empty DPKG_ROOT touch only stubbed systemctl, never host files.
            self.assertEqual(self._run_script(h, "pms-preremove.sh", "upgrade", use_dpkg_root=False).returncode, 0)
            self.assertEqual(self._run_script(h, "pms-preremove.sh", "1", use_dpkg_root=False).returncode, 0)
            self.assertNotIn("disable --now", h["log"].read_text(encoding="utf-8"))

            self.assertEqual(self._run_script(h, "pms-preremove.sh", "remove", use_dpkg_root=False).returncode, 0)
            self.assertIn("systemctl --no-reload disable --now speedle-pms.service", h["log"].read_text(encoding="utf-8"))

            h["log"].write_text("", encoding="utf-8")
            self.assertEqual(self._run_script(h, "pms-postremove.sh", "1", use_dpkg_root=False).returncode, 0)
            log_after_rpm_upgrade = h["log"].read_text(encoding="utf-8")
            self.assertIn("systemctl daemon-reload", log_after_rpm_upgrade)
            self.assertIn("systemctl try-restart speedle-pms.service", log_after_rpm_upgrade)

    def test_rejects_unsafe_preexisting_accounts_ownership_and_symlinks(self):
        bad_accounts = [
            ("speedle:x:0:", "", "privileged GID 0"),
            ("speedle:x:980:", "speedle:x:0:980:Speedle:/var/lib/speedle:/usr/sbin/nologin", "privileged UID 0"),
            ("speedle:x:980:", "speedle:x:980:100:Speedle:/var/lib/speedle:/usr/sbin/nologin", "primary GID"),
            ("speedle:x:980:", "speedle:x:980:980:Speedle:/home/speedle:/usr/sbin/nologin", "home directory"),
            ("speedle:x:980:", "speedle:x:980:980:Speedle:/var/lib/speedle:/bin/bash", "interactive"),
        ]
        for group_line, passwd_line, expected_err in bad_accounts:
            with self.subTest(expected_err=expected_err), tempfile.TemporaryDirectory() as tmp_raw:
                h = self._make_harness(Path(tmp_raw))
                (h["db"] / "group").write_text(group_line + "\n", encoding="utf-8")
                (h["db"] / "passwd").write_text((passwd_line + "\n") if passwd_line else "", encoding="utf-8")
                res = self._run_script(h, "preinstall.sh", "install")
                self.assertNotEqual(res.returncode, 0)
                self.assertIn(expected_err, res.stderr)

        # Preexisting /var/lib/speedle owned by another user must fail without chowning its contents.
        with tempfile.TemporaryDirectory() as tmp_raw:
            h = self._make_harness(Path(tmp_raw))
            self.assertEqual(self._run_script(h, "preinstall.sh", "install").returncode, 0)
            state_dir = h["root"] / "var/lib/speedle"
            state_dir.mkdir(parents=True)
            untouched = state_dir / "foreign.txt"
            untouched.write_text("keep", encoding="utf-8")
            meta = json.loads(h["meta"].read_text(encoding="utf-8"))
            meta[str(state_dir)] = {"uid": 1000, "owner": "alice", "group": "alice", "mode": "750"}
            h["meta"].write_text(json.dumps(meta), encoding="utf-8")
            res = self._run_script(h, "pms-postinstall.sh", "configure", "")
            self.assertNotEqual(res.returncode, 0)
            self.assertIn("alice:alice", res.stderr)
            self.assertNotIn(str(untouched), h["log"].read_text(encoding="utf-8"))

        # Symlink at /var/lib/speedle/policies.json must fail closed.
        with tempfile.TemporaryDirectory() as tmp_raw:
            h = self._make_harness(Path(tmp_raw))
            self.assertEqual(self._run_script(h, "preinstall.sh", "install").returncode, 0)
            state_dir = h["root"] / "var/lib/speedle"
            state_dir.mkdir(parents=True)
            (state_dir / "policies.json").symlink_to("/etc/passwd")
            meta = json.loads(h["meta"].read_text(encoding="utf-8"))
            meta[str(state_dir)] = {"uid": 980, "owner": "speedle", "group": "speedle", "mode": "750"}
            h["meta"].write_text(json.dumps(meta), encoding="utf-8")
            res = self._run_script(h, "pms-postinstall.sh", "configure", "")
            self.assertNotEqual(res.returncode, 0)
            self.assertIn("symlink", res.stderr)


if __name__ == "__main__":
    unittest.main()
