package transport

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
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

	// Fallback to sending request without creds.
	resp, err := t.inner.RoundTrip(r)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusOK {
		return resp, nil
	}

	// If response is 401 and there's an Authentication Realm, try to use
	// it to get a token. Dockerhub uses this for public images.
	if v := resp.Header.Get("Www-Authenticate"); resp.StatusCode == http.StatusUnauthorized && v != "" {
		// Check for an Authentication Realm in the response header.
		// https://docs.docker.com/registry/spec/auth/token/#how-to-authenticate
		realm, service, scope := parseWwwAuthenticate(v)

		// Didn't find an HTTP Authentication Realm to authorize at.
		if realm == "" {
			return resp, nil
		}

		tok, err := t.getToken(realm, service, scope)
		if err != nil {
			return nil, err
		}

		r.Header.Set("Authorization", "Bearer "+tok)
		// Don't cache this token, since tokens used from cache assume
		// "Basic" auth, and since the token might expire after some
		// amount of time. Instead, we'll go through the
		// WwwAuthenticate dance each time. :(
		// TODO: Allow this to be cached, with "Bearer" and expiration.
		return t.inner.RoundTrip(r)
	}

	return resp, nil
}

func parseWwwAuthenticate(v string) (realm, service, scope string) {
	v = v[strings.Index(v, " ")+1:]
	for {
		equal := strings.Index(v, "=")
		if equal == -1 {
			return
		}
		comma := strings.Index(v[equal:], ",")
		end := equal + comma
		if comma == -1 {
			end = len(v)
		}

		key := v[:equal]
		val := v[equal+2 : end-1] // strip ""s

		if key == "realm" {
			realm = val
		}
		if key == "service" {
			service = val
		}
		if key == "scope" {
			scope = val
		}

		if end == len(v) {
			return
		}
		v = v[equal+comma+1:]
	}
	panic("unreachable")
}

func (t *transport) getToken(realm, service, scope string) (string, error) {
	url := fmt.Sprintf("%s?service=%s&scope=%s", realm, service, scope)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	aresp, err := t.inner.RoundTrip(req)
	if err != nil {
		return "", err
	}

	if aresp.StatusCode == http.StatusOK {
		var tok struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(aresp.Body).Decode(&tok); err != nil {
			return "", err
		}
		return tok.Token, nil
	}
	return "", HTTPError{aresp}
}

// HTTPError represents an HTTP error encountered during authorizing the request.
type HTTPError struct {
	// Resp is the HTTP response returned by the server.
	Resp *http.Response
}

func (h HTTPError) Error() string {
	b, err := httputil.DumpResponse(h.Resp, true)
	if err != nil {
		return err.Error()
	}
	return fmt.Sprintf("HTTP error %d\n%s", h.Resp.StatusCode, string(b))
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
