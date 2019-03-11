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

// Package rebase provides methods to rebase images in a Docker registry.
package rebase

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// Rebaser provides a method for rebasing Docker images.
type Rebaser struct {
	keychain  authn.Keychain
	transport http.RoundTripper
}

// New returns a new Rebaser, using the specified keychain and HTTP transport.
func New(k authn.Keychain, t http.RoundTripper) Rebaser {
	return Rebaser{
		keychain:  k,
		transport: t,
	}
}

func (r Rebaser) get(s string) (v1.Image, error) {
	ref, err := name.ParseReference(s, name.WeakValidation)
	if err != nil {
		return nil, err
	}
	return remote.Image(ref, remote.WithAuthFromKeychain(r.keychain), remote.WithTransport(r.transport))
}

// Rebase constructs and pushes a new image based on orig, with layers from
// oldBase removed and replaced with those in newBase. The new image is pushed
// to the reference described by rebased.
func (r Rebaser) Rebase(origStr, oldBaseStr, newBaseStr, rebasedStr string) error {
	orig, err := r.get(origStr)
	if err != nil {
		return fmt.Errorf("could not get original image %q: %v", origStr, err)
	}
	origConfig, err := orig.ConfigFile()
	if err != nil {
		return fmt.Errorf("could not get config for original image %q: %v", origStr, err)
	}

	if oldBaseStr == "" && newBaseStr == "" {
		oldBaseStr, newBaseStr, err = getBasesFromLabel(origConfig.Config.Labels)
		if err != nil {
			return err
		}
		fmt.Println("Found LABEL rebase", oldBaseStr, newBaseStr)
	}

	oldBase, err := r.get(oldBaseStr)
	if err != nil {
		return fmt.Errorf("could not get old base image %q: %v", oldBaseStr, err)
	}
	newBase, err := r.get(newBaseStr)
	if err != nil {
		return fmt.Errorf("could not get new base image %q: %v", newBaseStr, err)
	}

	// rebasedStr must be a tag.
	rebasedRef, err := name.NewTag(rebasedStr, name.WeakValidation)
	if err != nil {
		return fmt.Errorf("could not parse rebased tag %q: %v", rebasedStr, err)
	}

	rebased, err := mutate.Rebase(orig, oldBase, newBase)
	if err != nil {
		return fmt.Errorf("error rebasing image: %v", err)
	}

	// Push the new rebased image.
	a, err := r.keychain.Resolve(rebasedRef.Context().Registry)
	if err != nil {
		return fmt.Errorf("could not authorize to %q: %v", rebasedRef.Context().Registry, err)
	}
	if err := remote.Write(rebasedRef, rebased, a, r.transport); err != nil {
		return fmt.Errorf("could not put new image %q: %v", rebasedStr, err)
	}

	return nil
}

func getBasesFromLabel(lbls map[string]string) (string, string, error) {
	if lbls == nil {
		return "", "", errors.New("Could not find LABEL indicating bases")
	}
	lbl, found := lbls["rebase"]
	if !found {
		return "", "", errors.New("Could not find LABEL indicating bases")
	}
	parts := strings.Split(lbl, " ")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("Malformed rebase LABEL: %s", lbl)
	}
	return parts[0], parts[1], nil
}
