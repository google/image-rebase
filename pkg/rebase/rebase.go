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

	"github.com/google/go-containerregistry/authn"
	"github.com/google/go-containerregistry/name"
	"github.com/google/go-containerregistry/v1"
	"github.com/google/go-containerregistry/v1/empty"
	"github.com/google/go-containerregistry/v1/mutate"
	"github.com/google/go-containerregistry/v1/remote"
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

func (r Rebaser) get(s string) (v1.Image, name.Reference, error) {
	ref, err := name.ParseReference(s, name.WeakValidation)
	if err != nil {
		return nil, nil, err
	}
	a, err := r.keychain.Resolve(ref.Context().Registry)
	if err != nil {
		return nil, nil, err
	}
	img, err := remote.Image(ref, a, r.transport)
	return img, ref, err
}

// Rebase constructs and pushes a new image based on orig, with layers from
// oldBase removed and replaced with those in newBase. The new image is pushed
// to the reference described by rebased.
func (r Rebaser) Rebase(origStr, oldBaseStr, newBaseStr, rebasedStr string) error {
	orig, origRef, err := r.get(origStr)
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

	oldBase, oldBaseRef, err := r.get(oldBaseStr)
	if err != nil {
		return fmt.Errorf("could not get old base image %q: %v", oldBaseStr, err)
	}
	newBase, newBaseRef, err := r.get(newBaseStr)
	if err != nil {
		return fmt.Errorf("could not get new base image %q: %v", newBaseStr, err)
	}

	// rebasedStr must be a tag, and the image doesn't exist yet.
	rebasedRef, err := name.NewTag(rebasedStr, name.WeakValidation)
	if err != nil {
		return fmt.Errorf("could not parse rebased tag %q: %v", rebasedStr, err)
	}

	// Verify that oldBase's layers are present in orig, otherwise orig is
	// not based on oldBase at all.
	origLayers, err := orig.Layers()
	if err != nil {
		return err
	}
	oldBaseLayers, err := oldBase.Layers()
	if err != nil {
		return err
	}
	if len(oldBaseLayers) > len(origLayers) {
		return fmt.Errorf("image %q is not based on %q", orig, oldBase)
	}
	for i, l := range oldBaseLayers {
		oldLayerDigest, _ := l.Digest()
		origLayerDigest, _ := origLayers[i].Digest()
		if oldLayerDigest != origLayerDigest {
			return fmt.Errorf("image %q is not based on %q", orig, oldBase)
		}
	}

	// Stitch together an image that contains:
	// - original image's config
	// - new base image's layers + top of original image's layers
	// - new base image's history + top of original image's history
	//
	// If new base image was specified by tag, write the "rebase" LABEL for
	// future automatic rebase detection.
	rebasedConfig := *origConfig.Config.DeepCopy()
	if _, ok := newBaseRef.(name.Tag); ok {
		dig, err := newBase.Digest()
		if err != nil {
			return fmt.Errorf("could not determine digest of %q: %v", newBaseRef, err)
		}
		newBaseDigRefStr := fmt.Sprintf("%s@%s", newBaseRef.Context(), dig)
		if _, err := name.NewDigest(newBaseDigRefStr, name.WeakValidation); err != nil {
			return fmt.Errorf("could not parse digest reference %q: %v", newBaseDigRefStr, err)
		}
		tag := newBaseRef.String()
		if rebasedConfig.Labels == nil {
			rebasedConfig.Labels = map[string]string{}
		}
		rebasedConfig.Labels["rebase"] = fmt.Sprintf("%s %s", newBaseDigRefStr, tag)
		fmt.Println("Adding LABEL rebase", rebasedConfig.Labels["rebase"])
	}
	rebasedImage, err := mutate.Config(empty.Image, rebasedConfig)
	if err != nil {
		return fmt.Errorf("failed to create empty image with original config: %v", err)
	}
	// Get new base layers and config for history.
	newBaseLayers, err := newBase.Layers()
	if err != nil {
		return fmt.Errorf("could not get new base layers for %q: %v", newBaseStr, err)
	}
	newConfig, err := newBase.ConfigFile()
	if err != nil {
		return fmt.Errorf("could not get config for new base image %q: %v", newBaseStr, err)
	}
	for i := range newBaseLayers {
		rebasedImage, err = mutate.Append(rebasedImage, mutate.Addendum{
			Layer:   newBaseLayers[i],
			History: newConfig.History[i],
		})
		if err != nil {
			return fmt.Errorf("failed to append layer %d of new base layers", i)
		}
	}
	for i := range origLayers[len(oldBaseLayers):] {
		rebasedImage, err = mutate.Append(rebasedImage, mutate.Addendum{
			Layer:   origLayers[i],
			History: origConfig.History[i],
		})
		if err != nil {
			return fmt.Errorf("failed to append layer %d of original layers", i)
		}
	}

	// Push the new rebased image.
	a, err := r.keychain.Resolve(rebasedRef.Context().Registry)
	if err != nil {
		return fmt.Errorf("could not authorize to %q: %v", rebasedRef.Context().Registry, err)
	}
	if err := remote.Write(rebasedRef, rebasedImage, a, r.transport, remote.WriteOptions{
		MountPaths: []name.Repository{origRef.Context(), oldBaseRef.Context(), newBaseRef.Context()},
	}); err != nil {
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
