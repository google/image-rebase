load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")
load("@io_bazel_rules_docker//go:image.bzl", "go_image")

go_image(
    name = "rebase",
    srcs = ["main.go"],
    deps = [
        "//pkg/rebase:go_default_library",
        "//vendor/golang.org/x/oauth2/google:go_default_library",
    ],
)

load("@io_bazel_rules_docker//container:container.bzl", "container_push")

container_push(
    name = "push",
    format = "Docker",
    image = ":rebase",
    registry = "gcr.io",
    repository = "test-argo/rebase",
    stamp = 1,
    tag = "latest",
)

load("@bazel_gazelle//:def.bzl", "gazelle")

gazelle(
    name = "gazelle",
    prefix = "github.com/GoogleCloudPlatform/image-rebase",
)

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    importpath = "github.com/GoogleCloudPlatform/image-rebase",
    visibility = ["//visibility:private"],
    deps = [
        "//pkg/rebase:go_default_library",
        "//vendor/golang.org/x/oauth2/google:go_default_library",
    ],
)

go_binary(
    name = "image-rebase",
    embed = [":go_default_library"],
    visibility = ["//visibility:public"],
)
