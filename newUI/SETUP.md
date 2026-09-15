third_party/libchdr setup
==========================

chdlib (CHD support) binds github.com/rtissera/libchdr via cgo. Add it
as a git submodule pinned to the commit this binding was verified
against, then build it once:

    git submodule add https://github.com/rtissera/libchdr.git third_party/libchdr
    cd third_party/libchdr
    git checkout 6cde5348eb118da3baf94f75a69577a005a484fd
    cd ../..
    git add third_party/libchdr .gitmodules
    git commit -m "vendor libchdr @ 6cde5348"

    ./scripts/build-libchdr.sh

After that, `go build ./...` works for any package importing chdlib
(discimg, transitively).

Build dependencies (see scripts/build-libchdr.sh's own header comment
for per-OS package names): cmake, a C compiler, zlib dev headers,
libzstd dev headers, libFLAC dev headers. lzma is vendored inside
libchdr itself and needs nothing extra.

Updating to a newer libchdr:

    cd third_party/libchdr
    git fetch
    git checkout <new commit>
    cd ../..
    ./scripts/build-libchdr.sh
    # re-run cassini's test suite against a few known CHDs before committing

CROSS-COMPILATION TRADE-OFF: this is the real cost of using the
official library instead of a from-scratch pure-Go implementation.
CGO_ENABLED=1 cross-compilation (e.g. building the Windows binary from
a Linux CI runner) requires a matching C cross-compiler (mingw-w64 for
Windows, osxcross or an actual macOS runner for macOS) available at
build time — plain `GOOS=windows GOARCH=amd64 go build` from Linux,
which works for the rest of cassini's pure-Go code, does NOT work for
chdlib/discimg without one. In practice this means CI needs either
native runners per target OS (GitHub Actions' windows-latest/
macos-latest, which is the simpler and more common choice) or a
maintained cross-compilation toolchain image. This is worth deciding
explicitly in CI setup (item 10 territory) rather than discovering it
when a release build silently fails to cross-compile.
