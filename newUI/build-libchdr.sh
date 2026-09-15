#!/usr/bin/env bash
# Copyright 2026 The cassini Authors
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Builds the vendored libchdr submodule (third_party/libchdr) as a
# static library for chdlib's cgo bindings to link against. Run this
# once after `git submodule update --init` and again after bumping the
# submodule to a newer libchdr commit.
#
# This is not a guess at what libchdr needs -- every flag here was run
# and verified to produce a working, tested build (see chdlib's doc
# comment for the compile+link+run smoke test that validated it)
# against libchdr commit 6cde5348eb118da3baf94f75a69577a005a484fd.
#
# Requires: cmake, a C compiler, zlib dev headers, libzstd dev headers,
# libFLAC dev headers. lzma is vendored by libchdr itself (deps/lzma-*)
# and needs no system package.
#
#   Debian/Ubuntu: apt-get install cmake zlib1g-dev libzstd-dev libflac-dev
#   Fedora:        dnf install cmake zlib-devel libzstd-devel flac-devel
#   macOS/brew:    brew install cmake zstd flac    (zlib ships with the OS)

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LIBCHDR_DIR="$ROOT/third_party/libchdr"
BUILD_DIR="$LIBCHDR_DIR/build"

if [ ! -f "$LIBCHDR_DIR/CMakeLists.txt" ]; then
	echo "error: $LIBCHDR_DIR is empty -- run 'git submodule update --init --recursive' first" >&2
	exit 1
fi

mkdir -p "$BUILD_DIR"
cmake -S "$LIBCHDR_DIR" -B "$BUILD_DIR" \
	-DWITH_SYSTEM_ZLIB=ON \
	-DWITH_SYSTEM_ZSTD=ON \
	-DBUILD_SHARED_LIBS=OFF \
	-DINSTALL_STATIC_LIBS=ON \
	-DCHDR_WANT_RAW_DATA_SECTOR=ON \
	-DCHDR_WANT_SUBCODE=ON \
	-DCMAKE_BUILD_TYPE=Release

cmake --build "$BUILD_DIR" --parallel

echo ""
echo "Built:"
echo "  $BUILD_DIR/libchdr-static.a"
echo "  $BUILD_DIR/deps/lzma-25.01/libchdr-lzma.a"
echo ""
echo "chdlib's cgo LDFLAGS point at these paths relative to \${SRCDIR} --"
echo "no further configuration needed. 'go build ./...' should now work"
echo "for any package importing chdlib."
