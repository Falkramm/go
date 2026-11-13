# Delve companion evidence

The local companion targets `go-delve/delve` commit `0b7d2120c4e9789394af65a2d87fc7f8ab08d6e8`, reporting version 1.27.2. It resolves `internal/runtime/maps.ctrlEmpty` through the target DWARF control type and uses the legacy 0x80 value when target metadata is absent. The companion patch SHA256 is `17df970b02413bd13176ea0acccd39d1ec8bd3a90bd50d9201deae3572d87d88`. [Patch](delve-compatibility.patch)

The companion includes unit coverage for five metadata situations and all 256 control values. Normal and race unit tests passed. Existing map evaluation tests passed with Go 1.27.4, pristine Go1.28-devel, and zero-empty Go1.28-devel fixture compilers across both map layouts.

The final pinned Go base had eight fresh targets covering pristine/patched, both group layouts, and debug/optimized builds. Compatible Delve and the updated Go GDB printer each passed all eight targets. Go1.28-devel compatibility probes use disabled Delve version checking because this pinned Delve officially supports through Go1.27. Go 1.27.4 target checks used enabled version checking.

GDB pretty-prints populated maps and checks indirect value boundaries. Empty/nil target states were checked structurally because the pristine printer has a recorded dereference limitation.

Upstream Delve review and release coordination are pending. This companion belongs to the Delve repository. Its source provenance and tested scope are recorded alongside the patch. [Metadata](delve-summary.json)
