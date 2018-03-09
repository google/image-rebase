/*
Copyright 2018 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package rebase

import (
	"reflect"
	"strings"
	"testing"
)

var digest = strings.Repeat("f", 64)

func TestImageName(t *testing.T) {
	for _, c := range []struct {
		in   string
		want *ImageName
	}{{
		in:   "gcr.io/proj/img:tag",
		want: &ImageName{"gcr.io", "proj/img", "tag", ""},
	}, {
		in:   "gcr.io/proj/img@sha256:" + digest,
		want: &ImageName{"gcr.io", "proj/img", "", "sha256:" + digest},
	}, {
		in:   "gcr.io/proj/img", // without tag or digest, assume :latest
		want: &ImageName{"gcr.io", "proj/img", "latest", ""},
	}, {
		in:   "ubuntu:latest",
		want: &ImageName{"index.docker.io", "ubuntu", "latest", ""},
	}, {
		in:   "ubuntu",
		want: &ImageName{"index.docker.io", "ubuntu", "latest", ""},
	}, {
		in:   "hostwithport:8080/image:foo",
		want: &ImageName{"hostwithport:8080", "image", "foo", ""},
	}, {
		in:   "totally incomprehensible",
		want: nil,
	}} {
		got := FromString(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("FromString(%q): got %+v, want %+v", c.in, got, c.want)
		}
	}
}
