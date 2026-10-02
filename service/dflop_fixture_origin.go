package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const VerificationFixtureVersion = "synthetic-v1"

// VerificationFixtureHandler exposes only the embedded synthetic allowlist.
// It has no upload, filesystem, task artifact, or user media access.
func VerificationFixtureHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.EscapedPath() != r.URL.Path {
		http.NotFound(w, r)
		return
	}
	for _, media := range DFLOPVerificationMediaInventory() {
		if ValidateDFLOPVerificationMedia(media) != nil {
			continue
		}
		expected := "/verification-fixtures/" + verificationFixtureCategory(media) + "/" + media.SHA256 + "/" + path.Base(media.Path)
		if r.URL.Path != expected || r.URL.RawQuery != "" {
			continue
		}
		data, err := DFLOPVerificationMediaBytes(media.ID)
		if err != nil || len(data) != media.Bytes || fmt.Sprintf("%x", sha256.Sum256(data)) != media.SHA256 {
			http.Error(w, "Fixture integrity failure", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", media.MIMEType)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("ETag", `"sha256-`+media.SHA256+`"`)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeContent(w, r, path.Base(media.Path), time.Time{}, bytes.NewReader(data))
		return
	}
	http.NotFound(w, r)
}

func DFLOPVerificationPublishedMedia(origin string) (map[string]VerificationMediaFixture, error) {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.Opaque != "" || parsed.Path != "" && parsed.Path != "/" {
		return nil, errors.New("FIXTURE_PUBLIC_HTTPS_ORIGIN_REQUIRED")
	}
	result := map[string]VerificationMediaFixture{}
	for _, media := range DFLOPVerificationMediaInventory() {
		if ValidateDFLOPVerificationMedia(media) != nil {
			continue
		}
		media.PublicURL = strings.TrimRight(origin, "/") + "/verification-fixtures/" + verificationFixtureCategory(media) + "/" + media.SHA256 + "/" + path.Base(media.Path)
		result[media.ID] = media
	}
	return result, nil
}

// VerifyDFLOPPublicMedia validates HEAD, GET, and byte-range reads. DNS is checked at dial
// time; redirects, credentials, private addresses, and response size overruns
// cannot turn the public fixture probe into a private-network fetch.
func VerifyDFLOPPublicMedia(ctx context.Context, media VerificationMediaFixture) error {
	parsed, err := url.Parse(media.PublicURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.Opaque != "" || media.Bytes <= 0 || media.Bytes > 16<<20 {
		return errors.New("FIXTURE_PUBLIC_URL_REQUIRED")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		if len(addresses) == 0 {
			return nil, errors.New("FIXTURE_PUBLIC_DNS_REQUIRED")
		}
		for _, address := range addresses {
			ip := address.IP
			if !verificationFixturePublicIP(ip) {
				return nil, errors.New("FIXTURE_PRIVATE_ADDRESS_REJECTED")
			}
		}
		dialer := net.Dialer{Timeout: 15 * time.Second}
		return dialer.DialContext(ctx, network, net.JoinHostPort(addresses[0].IP.String(), port))
	}
	defer transport.CloseIdleConnections()
	client := http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return verifyDFLOPPublicMediaResponses(ctx, media, &client)
}

func verifyDFLOPPublicMediaResponses(ctx context.Context, media VerificationMediaFixture, client *http.Client) error {
	probeClient := *client
	probeClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	var fullBody []byte
	rangeBytes := min(1024, media.Bytes)
	for _, probe := range []struct {
		method    string
		rangeRead bool
	}{
		{method: http.MethodHead},
		{method: http.MethodGet},
		{method: http.MethodGet, rangeRead: true},
	} {
		request, err := http.NewRequestWithContext(ctx, probe.method, media.PublicURL, nil)
		if err != nil {
			return err
		}
		request.Header.Set("Accept-Encoding", "identity")
		expectedStatus, expectedBytes := http.StatusOK, media.Bytes
		if probe.rangeRead {
			request.Header.Set("Range", "bytes=0-1023")
			expectedStatus, expectedBytes = http.StatusPartialContent, rangeBytes
		}
		response, err := probeClient.Do(request)
		if err != nil {
			return errors.New("FIXTURE_PUBLIC_REACHABILITY_REQUIRED")
		}
		if response.StatusCode != expectedStatus || response.ContentLength != int64(expectedBytes) ||
			response.Header.Get("Content-Type") != media.MIMEType ||
			response.Header.Get("Accept-Ranges") != "bytes" ||
			response.Header.Get("Cache-Control") != "public, max-age=31536000, immutable" ||
			response.Header.Get("Content-Encoding") != "" ||
			response.Header.Get("Content-Disposition") != "" {
			response.Body.Close()
			return errors.New("FIXTURE_PUBLIC_METADATA_MISMATCH")
		}
		if probe.rangeRead && response.Header.Get("Content-Range") != fmt.Sprintf("bytes 0-%d/%d", rangeBytes-1, media.Bytes) {
			response.Body.Close()
			return errors.New("FIXTURE_PUBLIC_RANGE_MISMATCH")
		}
		if probe.method == http.MethodHead {
			response.Body.Close()
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, int64(expectedBytes)+1))
		response.Body.Close()
		if readErr != nil || len(body) != expectedBytes {
			return errors.New("FIXTURE_PUBLIC_CHECKSUM_MISMATCH")
		}
		if probe.rangeRead {
			if !bytes.Equal(body, fullBody[:rangeBytes]) {
				return errors.New("FIXTURE_PUBLIC_RANGE_MISMATCH")
			}
			continue
		}
		if fmt.Sprintf("%x", sha256.Sum256(body)) != media.SHA256 {
			return errors.New("FIXTURE_PUBLIC_CHECKSUM_MISMATCH")
		}
		fullBody = body
	}
	return nil
}

func verificationFixtureCategory(media VerificationMediaFixture) string {
	if media.RequiresFace {
		return "rights-cleared-human-v1"
	}
	return VerificationFixtureVersion
}

func verificationFixturePublicIP(ip net.IP) bool {
	address, valid := netip.AddrFromSlice(ip)
	if !valid {
		return false
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsUnspecified() {
		return false
	}
	for _, network := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "::/96", "64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2001::/32", "2001:2::/48", "2001:10::/28", "2001:20::/28", "2001:db8::/32", "2002::/16", "3fff::/20", "5f00::/16", "fec0::/10"} {
		if netip.MustParsePrefix(network).Contains(address) {
			return false
		}
	}
	return true
}
