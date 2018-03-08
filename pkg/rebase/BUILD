load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = ["rebase.go"],
    importpath = "github.com/google/image-rebase/pkg/rebase",
    visibility = ["//visibility:public"],
)

go_test(
    name = "go_default_test",
    srcs = ["rebase_test.go"],
    embed = [":go_default_library"],
)
