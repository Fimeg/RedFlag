# Publication provenance

This branch is a constructed public projection. Its commit trailers identify
the internal source commit and the policy used to admit the published tree.

Reproduce the tree digest from a checked-out public commit with:

```sh
LC_ALL=C git ls-tree -r --full-tree HEAD^{tree} | LC_ALL=C sort | sha256sum
```

The result must equal the commit's `Tree-Digest` trailer.
