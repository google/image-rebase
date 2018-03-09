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
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/docker/distribution/reference"
)

const (
	// HTTP requests to the registry need to specify this value in the Accept
	// header.
	// https://docs.docker.com/registry/spec/manifest-v2-2/
	accept = "application/vnd.docker.distribution.manifest.v2+json"
)

// Rebaser provides a method for rebasing Docker images.
type Rebaser struct {
	Client *http.Client
}

// Rebase constructs and pushes a new image based on orig, with layers from
// oldBase removed and replaced with those in newBase. The new image is pushed
// to the reference described by rebased.
func (r Rebaser) Rebase(orig, oldBase, newBase, rebased *ImageName) error {
	if rebased.isDigest() {
		return fmt.Errorf("Rebased image cannot specify digest")
	}

	origData, err := r.getImageData(orig)
	if err != nil {
		return fmt.Errorf("GET original: %v", err)
	}

	oldData, err := r.getImageData(oldBase)
	if err != nil {
		return fmt.Errorf("GET old base: %v", err)
	}

	newData, err := r.getImageData(newBase)
	if err != nil {
		return fmt.Errorf("GET new base: %v", err)
	}

	// Verify that oldBase's layers are present in orig, otherwise orig is
	// not based on oldBase at all.
	for i, l := range oldData.manifest.Layers {
		if origData.manifest.Layers[i].Digest != l.Digest {
			return fmt.Errorf("%q is not based on %q", orig, oldBase)
		}
	}

	// If newBase is in another repository (within the same registry) as
	// rebased, we need to mount those layers into rebased's repository
	// first.
	if err := r.mount(rebased, newBase, newData.manifest); err != nil {
		return err
	}

	// Replace base layers, history and diff_ids.
	// TODO(jasonhall): Require that original image's top layers
	// includes a LABEL that marks it as a candidate for rebasing on oldBase.
	origData.manifest.Layers = append(newData.manifest.Layers, origData.manifest.Layers[len(oldData.manifest.Layers):]...)
	origData.config.History = append(newData.config.History, origData.config.History[len(oldData.config.History):]...)
	origData.config.RootFS.DiffIDs = append(newData.config.RootFS.DiffIDs, origData.config.RootFS.DiffIDs[len(oldData.config.RootFS.DiffIDs):]...)

	// Calculate new digest and size of config blob.
	h := sha256.New()
	b, err := origData.config.toJSON()
	if err != nil {
		return err
	}
	if _, err := io.Copy(h, bytes.NewReader(b)); err != nil {
		return err
	}
	origData.manifest.Config.Digest = fmt.Sprintf("sha256:%x", h.Sum(nil))
	origData.manifest.Config.Size = len(b)

	// PUT new config blob.
	if err := r.putBlob(rebased.reg, rebased.repo, origData.manifest.Config.Digest, origData.config); err != nil {
		return fmt.Errorf("POST new config blob: %v", err)
	}

	// PUT new manifest.
	if err := r.putManifest(rebased, origData.manifest); err != nil {
		return fmt.Errorf("PUT new manifest: %v", err)
	}

	return nil
}

type imageData struct {
	manifest manifest
	config   config
}

type manifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`
	Config        struct {
		MediaType string `json:"mediaType"`
		Size      int    `json:"size"`
		Digest    string `json:"digest"`
	} `json:"config"`
	Layers []struct {
		MediaType string `json:"mediaType"`
		Size      int    `json:"size"`
		Digest    string `json:"digest"`
	} `json:"layers"`
}

type config struct {
	allData map[string]interface{}
	History []struct {
		Created    string `json:"created"`
		CreatedBy  string `json:"created_by"`
		EmptyLayer bool   `json:"empty_layer,omitempty"`
	} `json:"history"`
	RootFS struct {
		DiffIDs []string `json:"diff_ids"`
		Type    string   `json:"type"`
	} `json:"rootfs"`
}

func configFromJSON(b []byte) (*config, error) {
	var allData map[string]interface{}
	if err := json.Unmarshal(b, &allData); err != nil {
		return nil, err
	}
	var c config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	c.allData = allData
	return &c, nil
}

func (c *config) toJSON() ([]byte, error) {
	c.allData["history"] = c.History
	c.allData["rootfs"] = c.RootFS
	return json.Marshal(c.allData)
}

// HTTPError represents an HTTP error encountered during rebasing.
type HTTPError struct {
	// Resp is the HTTP response returned by the server.
	Resp *http.Response
}

func (h HTTPError) Error() string {
	all, _ := ioutil.ReadAll(h.Resp.Body)
	h.Resp.Body.Close()
	h.Resp.Body = ioutil.NopCloser(bytes.NewReader(all))
	return fmt.Sprintf("HTTP error %d\n%s", h.Resp.StatusCode, string(all))
}

type ImageName struct {
	reg, repo, tag, dig string
}

func FromString(s string) *ImageName {
	ref, err := reference.Parse(s)
	if err != nil {
		log.Printf("Failed to parse image name: %v", err)
		return nil
	}
	named := ref.(reference.Named)
	reg, repo := reference.SplitHostname(named)
	if reg == "" {
		reg = "index.docker.io"
	}

	var tag, dig string
	tagged, ok := named.(reference.Tagged)
	if ok {
		tag = tagged.Tag()
		dig = ""
	} else {
		digested, ok := named.(reference.Digested)
		if ok {
			tag = ""
			dig = digested.Digest().String()
		} else {
			tag = "latest"
			dig = ""
		}
	}
	return &ImageName{reg, repo, tag, dig}
}

func (i ImageName) tagOrDigest() string {
	if i.tag != "" {
		return i.tag
	}
	return i.dig
}
func (i ImageName) isDigest() bool {
	return i.dig != ""
}

// "Get Manifest" from
// https://docs.docker.com/registry/spec/api/#pulling-an-image
func (r Rebaser) getImageData(name *ImageName) (*imageData, error) {
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", name.reg, name.repo, name.tagOrDigest())
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	resp, err := r.Client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, HTTPError{resp}
	}
	defer resp.Body.Close()
	var m manifest
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, err
	}
	// Next, look up the config file blob and decode it from JSON.
	config, err := r.getConfig(name.reg, name.repo, m.Config.Digest)
	if err != nil {
		return nil, err
	}
	return &imageData{m, *config}, nil
}

// "Get blob" from
// https://docs.docker.com/registry/spec/api/#blob
func (r Rebaser) getConfig(registry, repository, configDigest string) (*config, error) {
	url := fmt.Sprintf("https://%s/v2/%s/blobs/%s", registry, repository, configDigest)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.Client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, HTTPError{resp}
	}
	defer resp.Body.Close()
	all, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return configFromJSON(all)
}

// "Cross Repository Blob Mount" from
// https://docs.docker.com/registry/spec/api/#pushing-an-image
func (r Rebaser) mount(to, from *ImageName, m manifest) error {
	if to.repo == from.repo {
		// Nothing to cross-repository mount.
		return nil
	}
	if to.reg != from.reg {
		// TODO: Support cross-*registry* mounts by downloading and
		// re-uploading the blob to the new registry (i.e., mirroring).
		return fmt.Errorf("Cannot mount cross-registry from %s to %s", from.reg, to.reg)
	}

	for _, l := range m.Layers {
		url := fmt.Sprintf("https://%s/v2/%s/blobs/uploads/?mount=%s&from=%s", to.reg, to.repo, l.Digest, from.repo)
		req, err := http.NewRequest("POST", url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", accept)
		resp, err := r.Client.Do(req)
		if err != nil {
			return fmt.Errorf("Error mounting %s from %s to %s", l.Digest, from.repo, to.repo)
		}
		// If the blob is successfully mounted, registry will respond with 201 Created.
		// Otherwise, registry will fall back to standard upload and return 202 Accepted.
		// Either is acceptable.
		if resp.StatusCode == http.StatusAccepted {
			// This status might be returned if the digest or from
			// params are invalid, or if the registry doesn't
			// support cross-repo mounts.  TODO: Check whether the
			// digest exists in the from repository, to determine
			// whether the registry is telling us it doesn't
			// support cross-repo mounts.
			//
			// In either case, this indicates we need to download
			// then re-upload the blob to the new repository.
			// TODO: Download-then-reupload the blob to the new
			// repository.
			return HTTPError{resp}
		}
		if resp.StatusCode != http.StatusCreated {
			return HTTPError{resp}
		}
	}
	return nil
}

// "Monolithic upload" from
// https://docs.docker.com/registry/spec/api/#pushing-an-image
func (r Rebaser) putBlob(registry, repository, configDigest string, config config) error {
	b, err := config.toJSON()
	if err != nil {
		return err
	}
	// NB: This upload path is not supported by all registries.
	url := fmt.Sprintf("https://%s/v2/%s/blobs/uploads/?digest=%s", registry, repository, configDigest)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := r.Client.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusCreated {
		return HTTPError{resp}
	}
	return nil
}

// "Pushing an image manifest" from
// https://docs.docker.com/registry/spec/api/#pushing-an-image
func (r Rebaser) putManifest(name *ImageName, manifest manifest) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(manifest); err != nil {
		return err
	}
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", name.reg, name.repo, name.tag)
	req, err := http.NewRequest("PUT", url, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", accept)
	resp, err := r.Client.Do(req)
	if err != nil {
		return err
	}
	// API spec says it should return 201 Created, but we'll accept 200 OK just in case.
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return HTTPError{resp}
	}
	return nil
}
