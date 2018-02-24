# Docker Image Rebase

This builder image rewrites an image's manifest to replace layers in a base
image with layers in another version of that base image. It does so entirely
with API calls to the registry, so it doesn't have to download or upload full
layer blobs at any point.

*WARNING:* The image that results from such a rebase might not be valid in all
cases.

## Using `rebase`

TODO
