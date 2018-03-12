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

package main

import (
	"flag"
	"log"

	"github.com/google/image-rebase/pkg/rebase"
	"github.com/google/image-rebase/pkg/transport"
)

const scope = "https://www.googleapis.com/auth/devstorage.read_write"

var (
	orig    = flag.String("original", "", "Original image to rebase")
	oldBase = flag.String("old_base", "", "Old base to remove")
	newBase = flag.String("new_base", "", "New base to replace with")
	rebased = flag.String("rebased", "", "New rebased image tag to push") // Default to --original ?
)

func main() {
	flag.Parse()

	if *orig == "" {
		log.Fatal("Must specify --original")
	}
	if *rebased == "" {
		log.Fatal("Must specify --rebased")
	}

	r := rebase.Rebaser{transport.NewDockerCredsClient(nil)}
	if err := r.Rebase(
		rebase.FromString(*orig),
		rebase.FromString(*oldBase),
		rebase.FromString(*newBase),
		rebase.FromString(*rebased),
	); err != nil {
		log.Fatalf("Rebase: %v", err)
	}
}
