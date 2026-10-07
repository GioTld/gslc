"""Install an unpacked GSL distribution into a user-owned prefix."""
import argparse
import hashlib
import json
from pathlib import Path, PurePosixPath
import platform
import shutil
import tempfile


def install(bundle, prefix):
    if platform.system() != "Linux" or platform.machine() != "x86_64":
        raise ValueError("this distribution requires Linux x86_64")
    checksums = json.loads((bundle / "checksums.json").read_text())
    for name, expected in checksums.items():
        path = PurePosixPath(name)
        if path.is_absolute() or ".." in path.parts:
            raise ValueError("invalid distribution path")
        source = bundle / name
        if source.is_symlink() or hashlib.sha256(source.read_bytes()).hexdigest() != expected:
            raise ValueError(f"distribution checksum mismatch: {name}")
    command, library = prefix / "bin/gslc", prefix / "lib/gslc"
    if command.exists() or command.is_symlink() or library.exists() or library.is_symlink():
        raise ValueError("GSL is already installed at this prefix; use a new prefix or remove that installation first")
    command.parent.mkdir(parents=True, exist_ok=True)
    library.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".gslc-install-", dir=library.parent) as temporary:
        staged = Path(temporary) / "gslc"
        staged.mkdir()
        for name in checksums:
            if name.startswith("lib/gslc/"):
                target = staged / name[len("lib/gslc/"):]
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(bundle / name, target)
        (staged / "compiler").chmod(0o755)
        staged.rename(library)
        created = False
        try:
            with command.open("xb") as output:
                created = True
                output.write((bundle / "bin/gslc").read_bytes())
            command.chmod(0o755)
        except OSError:
            if created:
                command.unlink(missing_ok=True)
            shutil.rmtree(library)
            raise
    return command


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--prefix", type=Path, default=Path.home() / ".local")
    args = parser.parse_args()
    try:
        print(install(Path(__file__).resolve().parent, args.prefix.expanduser().resolve()))
    except (OSError, ValueError, KeyError) as error:
        parser.exit(1, f"gslc install: {error}\n")


if __name__ == "__main__":
    main()
