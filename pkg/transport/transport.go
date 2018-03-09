package transport

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
)

func NewDockerCredsClient(inner http.RoundTripper) *http.Client {
	if inner == nil {
		inner = http.DefaultTransport
	}
	return &http.Client{Transport: &transport{
		inner: inner,
		cache: map[string]string{},
	}}
}

type transport struct {
	inner http.RoundTripper
	cache map[string]string
}

func (t *transport) RoundTrip(r *http.Request) (*http.Response, error) {
	// Don't override an auth header if the request specified one.
	if r.Header.Get("Authorization") != "" {
		return t.inner.RoundTrip(r)
	}

	// If the auth token has already been used for this host, reuse it.
	if auth, found := t.cache[r.URL.Host]; found {
		r.Header.Set("Authorization", "Basic "+auth)
		return t.inner.RoundTrip(r)
	}

	dir := os.Getenv("HOME")
	if override := os.Getenv("DOCKER_CONFIG"); override != "" {
		dir = override
	}
	f, err := os.Open(path.Join(dir, ".docker/config.json"))
	if err != nil {
		return nil, fmt.Errorf("error opening ~/.docker/config.json: %v", err)
	}
	defer f.Close()

	var config struct {
		CredHelpers map[string]string `json:"credHelpers"`
		CredsStore  struct{}          `json:"credsStore"`
		Auths       map[string]struct {
			Auth string `json:"auth"`
		} `json:"auths"`
	}
	if err := json.NewDecoder(f).Decode(&config); err != nil {
		return nil, fmt.Errorf("error decoding ~/.docker/config.json: %v", err)
	}

	formats := []string{
		// naked domain
		"%s",
		// scheme-prefixed
		"http://%s",
		"https://%s",
		// scheme-prefixed with version in URL path
		"http://%s/v1/",
		"https://%s/v1/",
		"http://%s/v2/",
		"https://%s/v2/",
	}

	// Look for a matching credential helper and invoke it.
	for host, helper := range config.CredHelpers {
		for _, f := range formats {
			if fmt.Sprintf(f, r.URL.Host) == host {
				auth, err := invokeHelper(helper, fmt.Sprintf("https://%s", host))
				if err != nil {
					return nil, fmt.Errorf("error invoking credential helper for %q: %v", helper, err)
				}
				r.Header.Set("Authorization", "Basic "+auth)
				t.cache[r.URL.Host] = auth
				return t.inner.RoundTrip(r)
			}
		}
	}

	// TODO: Support credsStore.

	// Look for auths (base64-encoded username:password) and use it directly.
	for host, auth := range config.Auths {
		for _, f := range formats {
			if fmt.Sprintf(f, r.URL.Host) == host {
				r.Header.Set("Authorization", "Basic "+auth.Auth)
				t.cache[r.URL.Host] = auth.Auth
				return t.inner.RoundTrip(r)
			}
		}
	}

	// Fallback to sending request without creds. Hope it works!
	return t.inner.RoundTrip(r)
}

func invokeHelper(helper, serverURL string) (string, error) {
	cmd := exec.Command(fmt.Sprintf("docker-credential-%s", helper), "get")
	cmd.Stdin = strings.NewReader(serverURL)
	var buf bytes.Buffer
	cmd.Stdout = &buf

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("error executing helper: %v", err)
	}

	var output struct {
		Username string `json:"username"`
		Secret   string `json:"secret"`
	}
	if err := json.NewDecoder(&buf).Decode(&output); err != nil {
		return "", fmt.Errorf("error decoding output: %v", err)
	}
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", output.Username, output.Secret))), nil
}
