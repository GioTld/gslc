#!/usr/bin/env bash
set -euo pipefail

if command -v go >/dev/null 2>&1; then
    echo "Primary acceptance requires an environment without Go" >&2
    exit 1
fi

apt-get update -qq -o APT::Update::Error-Mode=any
DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    python3 binutils qemu-system-x86 seabios ipxe-qemu \
    clang-18=1:18.1.3-1ubuntu1 lld-18=1:18.1.3-1ubuntu1 \
    libllvm18=1:18.1.3-1ubuntu1 libclang1-18=1:18.1.3-1ubuntu1 \
    libclang-common-18-dev=1:18.1.3-1ubuntu1 \
    libclang-cpp18=1:18.1.3-1ubuntu1 llvm-18-linker-tools=1:18.1.3-1ubuntu1

ulimit -s 8192
export PYTHONDONTWRITEBYTECODE=1
python3 -m unittest discover -s test/bootstrap -p 'test_*.py' -v
python3 scripts/gsl.py bootstrap --output "$1/initial"
python3 scripts/gsl.py test --compiler "$1/initial/compiler" \
    --clang clang-18 --linker ld.lld-18 --llvm-version 18.1.3 \
    --output "$1/acceptance"
