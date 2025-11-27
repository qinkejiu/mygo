This directory will host the vendored subset of LLGo required by MyGO.

Expected layout:

```
third_party/llgo/
└── packages/    # Copy of llgo/internal/packages
```

Run `scripts/update-llgo.sh` (to be added later) to refresh the contents once the toolchain is available.
