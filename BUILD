load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")
load("@bazel_gazelle//:def.bzl", "gazelle")

gazelle(
    name = "gazelle",
    prefix = "github.com/google/image-rebase",
)

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    importpath = "github.com/google/image-rebase",
    visibility = ["//visibility:private"],
    deps = [
        "//pkg/rebase:go_default_library",
        "//vendor/github.com/google/go-containerregistry/authn:go_default_library",
    ],
)

go_binary(
    name = "image-rebase",
    embed = [":go_default_library"],
    pure = "on",
    visibility = ["//visibility:public"],
)

load("@io_bazel_rules_docker//go:image.bzl", "go_image")

go_image(
    name = "image",
    binary = ":image-rebase",
)
