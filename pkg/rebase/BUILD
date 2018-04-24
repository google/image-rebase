load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["rebase.go"],
    importpath = "github.com/google/image-rebase/pkg/rebase",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/google/go-containerregistry/authn:go_default_library",
        "//vendor/github.com/google/go-containerregistry/name:go_default_library",
        "//vendor/github.com/google/go-containerregistry/v1:go_default_library",
        "//vendor/github.com/google/go-containerregistry/v1/empty:go_default_library",
        "//vendor/github.com/google/go-containerregistry/v1/mutate:go_default_library",
        "//vendor/github.com/google/go-containerregistry/v1/remote:go_default_library",
    ],
)
