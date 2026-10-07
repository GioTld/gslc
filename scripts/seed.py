"""Verify and reproduce local GSL seed bundles using Python's standard library."""
import argparse
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import platform
import shutil
import subprocess
import sys
import tarfile
import tempfile


class SeedError(Exception):
    pass


def sha(data):
    return hashlib.sha256(data).hexdigest()


def read_json(path):
    try:
        result = json.loads(path.read_text())
        if not isinstance(result, dict):
            raise SeedError(f"JSON object required: {path}")
        return result
    except (OSError, ValueError) as error:
        raise SeedError(f"cannot read {path}: {error}") from error


def relative(name):
    if not isinstance(name, str) or not name:
        raise SeedError("invalid source/artifact path")
    path = PurePosixPath(name)
    if path.is_absolute() or ".." in path.parts or str(path) != name:
        raise SeedError(f"invalid source/artifact path: {name}")
    return name


def source_members(bundle, hashes, destination=None):
    if not isinstance(hashes, dict) or not hashes:
        raise SeedError("missing source hashes")
    seen = set()
    with tarfile.open(bundle / "sources.tar", "r:") as archive:
        for member in archive:
            name = relative(member.name)
            if not member.isfile() or name in seen or name not in hashes:
                raise SeedError(f"unexpected source archive member: {name}")
            data = archive.extractfile(member).read()
            if sha(data) != hashes[name]:
                raise SeedError(f"source checksum mismatch: {name}")
            seen.add(name)
            if destination is not None:
                output = destination / name
                output.parent.mkdir(parents=True, exist_ok=True)
                output.write_bytes(data)
    if seen != set(hashes):
        raise SeedError("source archive is missing recorded files")


def verify(bundle, policy, expected=None):
    if platform.system() != "Linux" or platform.machine() != "x86_64":
        raise SeedError("seed workflow requires Linux x86_64")
    metadata = read_json(bundle / "seed.json")
    if metadata.get("format") == 1:
        for key, value in policy["legacy_import"].items():
            if metadata.get(key) != value:
                raise SeedError("legacy seed does not match the recorded recovery identity")
        host = metadata.get("host") == "x86_64 Linux"
    elif metadata.get("format") == 2:
        host = metadata.get("host") == policy["host"]
        if metadata.get("source_profile") != policy["source_profile"]:
            raise SeedError("incompatible compiler source profile")
        if metadata.get("source_revision") != "sha256:" + str(metadata.get("source_snapshot_sha256")):
            raise SeedError("source revision does not identify its archive")
    else:
        raise SeedError("unsupported seed format")
    if not host or metadata.get("targets") != policy["targets"]:
        raise SeedError("incompatible seed host or target profiles")
    artifacts = metadata.get("artifacts")
    if not isinstance(artifacts, dict) or not artifacts:
        raise SeedError("missing artifact hashes")
    for name, digest in artifacts.items():
        relative(name)
        if "/" in name or (bundle / name).is_symlink():
            raise SeedError(f"invalid artifact: {name}")
        if sha((bundle / name).read_bytes()) != digest:
            raise SeedError(f"artifact checksum mismatch: {name}")
    for name, key in [("compiler", "compiler_sha256"), ("sources.tar", "source_snapshot_sha256")]:
        if artifacts.get(name) != metadata.get(key):
            raise SeedError(f"missing or inconsistent identity: {name}")
    if expected is not None and metadata["compiler_sha256"] != expected:
        raise SeedError("compiler does not match expected identity")
    compiler = (bundle / "compiler").read_bytes()
    if len(compiler) < 20 or compiler[:6] != b"\x7fELF\x02\x01" or compiler[18:20] != b"\x3e\x00":
        raise SeedError("compiler is not an x86_64 ELF executable")
    if not os.access(bundle / "compiler", os.X_OK):
        raise SeedError("seed compiler is not executable")
    source_members(bundle, metadata.get("source_files"))
    if metadata["format"] == 2:
        for name in ["inputs.json", "candidate-b", "candidate-c", "candidate-b.ll", "candidate-c.ll"]:
            if name not in artifacts:
                raise SeedError(f"missing fixed-point artifact: {name}")
        if read_json(bundle / "inputs.json") != metadata.get("native_inputs"):
            raise SeedError("native input records disagree")
        if artifacts["candidate-b"] != artifacts["candidate-c"] or artifacts["candidate-b"] != metadata["compiler_sha256"]:
            raise SeedError("compiler executable fixed point disagrees")
        if artifacts["candidate-b.ll"] != artifacts["candidate-c.ll"]:
            raise SeedError("compiler IR fixed point disagrees")
    return metadata


def run(args, cwd):
    try:
        result = subprocess.run(args, cwd=cwd, capture_output=True, text=True, timeout=60)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise SeedError(f"command failed: {args[0]}: {error}") from error
    if result.returncode:
        raise SeedError(f"command failed ({result.returncode}): {' '.join(map(str, args))}\n{result.stdout}{result.stderr}")
    return result.stdout


def executable(name):
    path = shutil.which(name)
    if not path:
        raise SeedError(f"required executable missing: {name}")
    return str(Path(path).absolute())


def snapshot(root, output):
    files = []
    extensions = {".go", ".gsl", ".json", ".yml", ".yaml", ".S", ".ld", ".py"}
    for directory in ["cmd", "pkg", "lib", "test", ".github", "scripts"]:
        files.extend(p for p in (root / directory).rglob("*") if p.is_file() and p.suffix in extensions)
    files.extend(root / name for name in ["go.mod", "go.sum", "README.md", "spec.md"] if (root / name).is_file())
    hashes = {}
    with tarfile.open(output, "w") as archive:
        for path in sorted(set(files)):
            if path.is_symlink():
                raise SeedError(f"source symlink is unsupported: {path}")
            name = relative(path.relative_to(root).as_posix())
            data = path.read_bytes()
            hashes[name] = sha(data)
            member = tarfile.TarInfo(name)
            member.size, member.mode = len(data), 0o644
            archive.addfile(member, io.BytesIO(data))
    return hashes


def create(args, policy, seed, previous):
    output = args.output.resolve()
    if output.exists():
        raise SeedError(f"output already exists: {output}")
    output.parent.mkdir(parents=True, exist_ok=True)
    work = Path(tempfile.mkdtemp(prefix=output.name + ".building-", dir=output.parent))
    try:
        hashes = snapshot(args.root.resolve(), work / "sources.tar")
        source = work / "source"
        source_members(work, hashes, source)
        manifest = read_json(source / "test/bootstrap/manifest.json")
        entry, ordered = relative(manifest["entry"]), manifest["source_files"]
        if not ordered or any(relative(name) not in hashes for name in ordered) or entry not in ordered:
            raise SeedError("invalid compiler source closure")
        clang = executable(args.clang)
        linker = executable(args.linker or run([clang, "--print-prog-name=ld"], source).strip())
        versions = {"clang": run([clang, "--version"], source), "linker": run([linker, "--version"], source)}
        flags = ["-O2", "--target=x86_64-unknown-linux-gnu", "-Wl,--build-id=none", "--ld-path=" + linker]
        prior = seed / "compiler"
        for stage in ["candidate-a", "candidate-b", "candidate-c"]:
            print(f"Building {stage} from {prior.name}", flush=True)
            run([str(prior), entry, str(work / "native.ll")], source)
            shutil.copyfile(work / "native.ll", work / (stage + ".ll"))
            run([clang, *flags, str(work / "native.ll"), "-o", str(work / stage)], source)
            prior = work / stage
        if (work / "candidate-b.ll").read_bytes() != (work / "candidate-c.ll").read_bytes():
            raise SeedError("compiler IR fixed point failed")
        if (work / "candidate-b").read_bytes() != (work / "candidate-c").read_bytes():
            raise SeedError("compiler executable fixed point failed")
        shutil.copy2(work / "candidate-b", work / "compiler")
        archive_sha = sha((work / "sources.tar").read_bytes())
        inputs = {**versions, "clang_executable": clang, "linker_executable": linker, "flags": flags,
                  "source_files": ordered, "sha256": {name: hashes[name] for name in ordered}}
        (work / "inputs.json").write_text(json.dumps(inputs, indent=2) + "\n")
        artifact_hashes = {p.name: sha(p.read_bytes()) for p in work.iterdir() if p.is_file()}
        metadata = {"format": 2, "host": policy["host"], "targets": policy["targets"],
                    "source_profile": policy["source_profile"], "source_revision": "sha256:" + archive_sha,
                    "source_snapshot_sha256": archive_sha, "compiler_sha256": artifact_hashes["compiler"],
                    "source_files": hashes, "artifacts": artifact_hashes, "native_inputs": inputs,
                    "previous_seed": {"compiler_sha256": previous["compiler_sha256"],
                                      "source_revision": "sha256:" + previous["source_snapshot_sha256"]},
                    "verified": {"compiler_ir_fixed_point": True, "compiler_binary_fixed_point": True},
                    "acceptance_scope": "seed reconstruction and fixed point; full fixture acceptance remains 12B",
                    "recovery": "extract sources.tar; use the recorded previous seed to build A, then A to build B and B to build C with native_inputs; compare B/C IR and bytes"}
        (work / "seed.json").write_text(json.dumps(metadata, indent=2) + "\n")
        shutil.rmtree(source)
        verify(work, policy)
        work.rename(output)
        print(json.dumps({"bundle": str(output), "compiler_sha256": metadata["compiler_sha256"],
                          "source_revision": metadata["source_revision"]}, indent=2))
    except Exception as error:
        raise SeedError(f"{error}\nFailure artifacts retained: {work}") from error


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--policy", type=Path, default=Path(__file__).resolve().parents[1] / "test/bootstrap/seed-policy.json")
    subparsers = parser.add_subparsers(dest="command", required=True)
    for command in ["verify", "create"]:
        sub = subparsers.add_parser(command)
        sub.add_argument("--seed", type=Path)
        sub.add_argument("--expected-compiler")
        if command == "create":
            sub.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[1])
            sub.add_argument("--output", type=Path, required=True)
            sub.add_argument("--clang", default="clang")
            sub.add_argument("--linker")
    args = parser.parse_args(argv)
    try:
        policy = read_json(args.policy)
        if policy.get("format") != 1:
            raise SeedError("unsupported seed policy format")
        selected = args.seed or os.environ.get("GSL_SEED")
        if not selected:
            raise SeedError("missing seed: supply --seed or GSL_SEED; no Go fallback")
        seed = Path(selected).expanduser().resolve()
        metadata = verify(seed, policy, args.expected_compiler)
        if args.command == "verify":
            print(json.dumps({"bundle": str(seed), "compiler_sha256": metadata["compiler_sha256"],
                              "source_revision": "sha256:" + metadata["source_snapshot_sha256"]}, indent=2))
        else:
            create(args, policy, seed, metadata)
        return 0
    except (SeedError, OSError, ValueError, KeyError, TypeError, tarfile.TarError) as error:
        print(f"seed: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
