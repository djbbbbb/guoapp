package core

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNativeStreamMP4IgnoresPlaylistWordsInURL(t *testing.T) {
	mediaBytes := []byte{0, 0, 0, 16, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm', 0, 0, 0, 0}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		if r.Method == http.MethodGet {
			_, _ = w.Write(mediaBytes)
		}
	}))
	defer upstream.Close()
	engine, err := newNativeEngine(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stream, err := newNativeStreamServer(engine.downloader)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.server.Close()
	for _, sourcePath := range []string{
		"/video.mp4?source=hls",
		"/video.mp4?manifest=index.m3u8",
		"/api/m3u8/hls/video.mp4?manifest=index.m3u8&source=hls",
	} {
		t.Run(sourcePath, func(t *testing.T) {
			address, token := stream.nativeOpen(providerMedia{URL: upstream.URL + sourcePath})
			defer stream.nativeRelease(token)
			if !strings.HasSuffix(address, ".mp4") {
				t.Fatalf("MP4 entry was labeled as a playlist: %s", address)
			}
			response, err := http.Get(address)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "video/mp4" || !bytes.Equal(body, mediaBytes) {
				t.Fatalf("MP4 bytes were not preserved: status=%d type=%q body=%x", response.StatusCode, response.Header.Get("Content-Type"), body)
			}
		})
	}
}

func TestNativeStreamHLSKeyDoesNotRelabelMediaAssets(t *testing.T) {
	key := []byte("0123456789abcdef")
	initBytes := []byte{0, 0, 0, 16, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm', 0, 0, 0, 0}
	segmentBytes := []byte{0x47, 0x40, 0, 0x10, 0, 0xff, 0x80, 0x01}
	playlist := "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:2\n#EXT-X-KEY:METHOD=AES-128,URI=\"/source.key\",IV=0x00000000000000000000000000000001\n#EXT-X-MAP:URI=\"/api/m3u8/init.mp4?source=hls\"\n#EXTINF:2,\n/api/m3u8/segment.ts?source=hls&manifest=index.m3u8\n#EXT-X-ENDLIST\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/root":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = io.WriteString(w, playlist)
		case "/api/m3u8/init.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write(initBytes)
		case "/api/m3u8/segment.ts":
			w.Header().Set("Content-Type", "video/mp2t")
			_, _ = w.Write(segmentBytes)
		case "/source.key":
			t.Error("source key was requested instead of the resolved key")
			w.WriteHeader(http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	engine, err := newNativeEngine(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stream, err := newNativeStreamServer(engine.downloader)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.server.Close()
	for _, cached := range []bool{false, true} {
		name := "fetched"
		if cached {
			name = "cached"
		}
		t.Run(name, func(t *testing.T) {
			media := providerMedia{URL: upstream.URL + "/root", HLSKey: key}
			if cached {
				media.Playlist = playlist
			}
			address, token := stream.nativeOpen(media)
			defer stream.nativeRelease(token)
			if !strings.HasSuffix(address, ".m3u8") {
				t.Fatalf("HLS key entry lost its playlist type: %s", address)
			}
			read := func(address, contentType string) []byte {
				t.Helper()
				response, err := http.Get(address)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				body, err := io.ReadAll(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != contentType {
					t.Fatalf("asset type was not preserved: status=%d type=%q body=%q", response.StatusCode, response.Header.Get("Content-Type"), body)
				}
				return body
			}
			body := string(read(address, "application/vnd.apple.mpegurl"))
			matches := nativePlaylistURI.FindAllStringSubmatch(body, -1)
			if len(matches) != 2 {
				t.Fatalf("missing rewritten key or init: %s", body)
			}
			if !bytes.Equal(read(matches[0][1], "application/octet-stream"), key) {
				t.Fatal("resolved key bytes changed")
			}
			if !bytes.Equal(read(matches[1][1], "video/mp4"), initBytes) {
				t.Fatal("initialization MP4 bytes changed")
			}
			segment := ""
			for _, line := range strings.Split(body, "\n") {
				if strings.HasPrefix(line, "http://") {
					segment = line
					break
				}
			}
			if segment == "" {
				t.Fatal("missing rewritten segment")
			}
			if !bytes.Equal(read(segment, "video/mp2t"), segmentBytes) {
				t.Fatal("media segment bytes changed")
			}
		})
	}
}
