# Docker Image Rebase

## This repo is obsolete, see
[go-containerregistry](https://godoc.org/github.com/google/go-containerregistry)'s
[`mutate.Rebase`](https://godoc.org/github.com/google/go-containerregistry/pkg/v1/mutate#Rebase)
instead.

That repo also contains code for a CLI to rebase container images, [`crane
rebase`](https://github.com/google/go-containerregistry/blob/master/cmd/crane/doc/crane_rebase.md),
which is also [described in more detail
here](https://github.com/google/go-containerregistry/blob/master/cmd/crane/rebase.md).

This package only exists as a thin wrapper around that functionality, which is
much more flexible, and well-tested.

[![Build Status](https://travis-ci.org/google/image-rebase.svg?branch=master)](https://travis-ci.org/google/image-rebase)

### This is not an official Google product.

### This code is experimental and might break you if not used correctly.
