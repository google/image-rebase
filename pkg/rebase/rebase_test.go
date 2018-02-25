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
	"testing"
)

func TestImageName(t *testing.T) {
	for _, c := range []struct {
		in      string
		want    *ImageName
		wantTag bool
	}{{
		in:      "gcr.io/proj/img:tag",
		want:    &ImageName{"gcr.io", "proj/img", tag("tag")},
		wantTag: true,
	}, {
		in:      "gcr.io/proj/img@sha256:gobbledegook",
		want:    &ImageName{"gcr.io", "proj/img", digest("sha256:gobbledegook")},
		wantTag: false,
	}, {
		in:   "gcr.io/proj/img", // without tag or digest
		want: nil,
	}, {
		in:   "totally incomprehensible",
		want: nil,
	}} {
		got := FromString(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("FromString(%q): got %+v, want %+v", c.in, got, c.want)
		}
		if c.wantTag && (!got.IsTag() || got.IsDigest()) {
			t.Errorf("FromString(%q): got non-tag image name, wanted tag", c.in)
		}
		if got != nil {
			if !c.wantTag && (got.IsTag() || !got.IsDigest()) {
				t.Errorf("FromString(%q): got tag image name, wanted non-tag", c.in)
			}
			if got.IsTag() == got.IsDigest() {
				t.Errorf("FromString(%q) IsTag(%t) == IsDigest(%t)", c.in, got.IsTag(), got.IsDigest())
			}
		}
	}
}
