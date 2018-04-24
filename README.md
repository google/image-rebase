# Docker Image Rebase

This builder rewrites an image's manifest to replace layers in a base image
with layers in another version of that base image. It does so entirely with API
calls to the registry, so it doesn't have to download or upload full layer
blobs at any point.

*WARNING:* The image that results from such a rebase might not be valid in all
cases.

## Using `rebase`

For example, to rebase the `ubuntu` base image for an image `image:latest` and
tag the newly rebased image as `image:rebased`, determine the digest of the
original base image and run the following build step:

```yaml
steps:
- name: 'gcr.io/$PROJECT_ID/rebase'
  args:
  - --original=gcr.io/$PROJECT_ID/image:latest
  - --old_base=ubuntu@sha256:<digest>
  - --new_base=ubuntu:latest
  - --rebased=gcr.io/$PROJECT_ID/image:rebased

# No `images:` since the image is modified in the registry.
```

You can determine base image digests using `gcloud alpha container images
describe <image> --show-image-basis`

### This is not an official Google product
