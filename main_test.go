package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/router"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocumentationDirectNavigation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("FRONTEND_BASE_URL", "")
	engine := newHTTPRouter()
	router.SetRouter(engine, router.WebAssets{BuildFS: buildFS, IndexPage: indexPage})
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	client := server.Client()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	for _, page := range []string{"", "quickstart", "models", "pricing", "text-chat", "image", "video", "audio"} {
		t.Run("docs/"+page, func(t *testing.T) {
			path := "/docs"
			file := "web/dist/docs/index.html"
			if page != "" {
				path += "/" + page
				file = "web/dist" + path + "/index.html"
			}
			expected, err := buildFS.ReadFile(file)
			require.NoError(t, err)
			response, err := client.Get(server.URL + path)
			require.NoError(t, err)
			assert.Equal(t, http.StatusMovedPermanently, response.StatusCode)
			location := page + "/"
			if page == "" {
				location = "docs/"
			}
			assert.Equal(t, location, response.Header.Get("Location"))
			require.NoError(t, response.Body.Close())

			response, err = client.Get(server.URL + path + "/")
			require.NoError(t, err)
			body, err := io.ReadAll(response.Body)
			require.NoError(t, response.Body.Close())
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, response.StatusCode)
			assert.Empty(t, response.Header.Get("Location"))
			t.Logf("GET %s -> %d Location: %q; GET %s/ -> %d, exact SSG HTML", path, http.StatusMovedPermanently, location, path, response.StatusCode)
			assert.Contains(t, response.Header.Get("Content-Type"), "text/html")
			assert.True(t, bytes.Equal(expected, body), "must serve prerendered HTML")

			followed, err := http.Get(server.URL + path)
			require.NoError(t, err, "direct navigation must finish without a redirect loop")
			assert.Equal(t, http.StatusOK, followed.StatusCode)
			followedBody, err := io.ReadAll(followed.Body)
			require.NoError(t, followed.Body.Close())
			require.NoError(t, err)
			assert.True(t, bytes.Equal(expected, followedBody), "direct navigation must return SSG HTML")
		})
	}

	t.Run("docs preserves query parameters", func(t *testing.T) {
		response, err := http.Get(server.URL + "/docs/quickstart?lang=vi")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, "/docs/quickstart/", response.Request.URL.Path)
		assert.Equal(t, "lang=vi", response.Request.URL.RawQuery)
		require.NoError(t, response.Body.Close())
	})

	for _, path := range []string{"/", "/dashboard/overview", "/dashboard/overview/"} {
		t.Run(path, func(t *testing.T) {
			response, err := client.Get(server.URL + path)
			require.NoError(t, err)
			body, err := io.ReadAll(response.Body)
			require.NoError(t, response.Body.Close())
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, response.StatusCode)
			assert.Empty(t, response.Header.Get("Location"))
			assert.Equal(t, string(indexPage), string(body))
		})
	}
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/api/notice", http.StatusOK},
		{"/api/does-not-exist", http.StatusNotFound},
		{"/v1/does-not-exist", http.StatusNotFound},
	} {
		t.Run(tc.path, func(t *testing.T) {
			response, err := client.Get(server.URL + tc.path)
			require.NoError(t, err)
			assert.Equal(t, tc.status, response.StatusCode)
			assert.Contains(t, response.Header.Get("Content-Type"), "application/json")
			assert.Empty(t, response.Header.Get("Location"))
			require.NoError(t, response.Body.Close())
		})
	}
}
