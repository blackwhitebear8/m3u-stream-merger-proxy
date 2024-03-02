package m3u

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"m3u-stream-merger/database"
)

func TestGenerateM3UContent(t *testing.T) {
	// Define a sample stream for testing
	stream := database.StreamInfo{
		TvgID:    "1",
		Title:    "TestStream",
		LogoURL:  "http://example.com/logo.png",
		Group:    "TestGroup",
		URLs:     []database.StreamURL{{Content: "http://example.com/stream"}},
	}

  sqliteDBPath := filepath.Join(".", "data", "database.sqlite")

  // Test InitializeSQLite and check if the database file exists
  err := database.InitializeSQLite()
  if err != nil {
      t.Errorf("InitializeSQLite returned error: %v", err)
  }
  defer os.Remove(sqliteDBPath) // Cleanup the database file after the test

  _, err = database.InsertStream(stream)
  if err != nil {
    t.Fatal(err)
  }

	// Create a new HTTP request
	req, err := http.NewRequest("GET", "/generate", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create a ResponseRecorder to record the response
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(GenerateM3UContent)

	// Call the ServeHTTP method of the handler to execute the test
	handler.ServeHTTP(rr, req)

	// Check the status code
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	// Check the Content-Type header
	expectedContentType := "text/plain"
	if contentType := rr.Header().Get("Content-Type"); contentType != expectedContentType {
		t.Errorf("handler returned unexpected Content-Type: got %v want %v",
			contentType, expectedContentType)
	}

	// Check the generated M3U content
	expectedContent := fmt.Sprintf(`#EXTM3U
#EXTINF:-1 tvg-id="1" tvg-name="TestStream" tvg-logo="http://example.com/logo.png" group-title="TestGroup",TestStream
%s`, generateStreamURL("http:///stream", "TestStream"))
	if rr.Body.String() != expectedContent {
		t.Errorf("handler returned unexpected body: got %v want %v",
			rr.Body.String(), expectedContent)
	}
}

func TestParseM3UFromURL(t *testing.T) {
	testM3UContent := `
#EXTM3U
#EXTINF:-1 tvg-id="bbc1" tvg-name="BBC One" group-title="UK",BBC One
http://example.com/bbc1
#EXTINF:-1 tvg-id="bbc2" tvg-name="BBC Two" group-title="UK",BBC Two
http://example.com/bbc2
#EXTINF:-1 tvg-id="cnn" tvg-name="CNN International" group-title="News",CNN
http://example.com/cnn
#EXTVLCOPT:logo=http://example.com/bbc_logo.png
#EXTINF:-1 tvg-id="fox" tvg-name="FOX" group-title="Entertainment",FOX
http://example.com/fox
`
	// Create a mock HTTP server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		fmt.Fprintln(w, testM3UContent)
	}))
	defer mockServer.Close()

  sqliteDBPath := filepath.Join(".", "data", "database.sqlite")

  // Test InitializeSQLite and check if the database file exists
  err := database.InitializeSQLite()
  if err != nil {
      t.Errorf("InitializeSQLite returned error: %v", err)
  }
  defer os.Remove(sqliteDBPath) // Cleanup the database file after the test

	// Test the parseM3UFromURL function with the mock server URL
	err = ParseM3UFromURL(mockServer.URL, 0)
	if err != nil {
		t.Errorf("Error parsing M3U from URL: %v", err)
	}

  // Verify expected values
	expectedStreams := []database.StreamInfo{
		{Title: "BBC One", TvgID: "bbc1", Group: "UK", URLs: []database.StreamURL{
      {
        Content: "http://example.com/bbc1",  
      },
    }},
		{Title: "BBC Two", TvgID: "bbc2", Group: "UK", URLs: []database.StreamURL{
      {
        Content: "http://example.com/bbc2",  
      },
    }},
		{Title: "CNN International", TvgID: "cnn", Group: "News", URLs: []database.StreamURL{
      {
        Content: "http://example.com/cnn",  
      },
    }},
		{Title: "FOX", TvgID: "fox", Group: "Entertainment", URLs: []database.StreamURL{
      {
        Content: "http://example.com/fox",  
      },
    }},
	}

	storedStreams, err := database.GetStreams()
	if err != nil {
		t.Fatalf("Error retrieving streams from database: %v", err)
	}

	// Compare the retrieved streams with the expected streams
	if len(storedStreams) != len(expectedStreams) {
		t.Fatalf("Expected %d streams, but got %d", len(expectedStreams), len(storedStreams))
	}

	for i, expected := range expectedStreams {
		if !streamInfoEqual(storedStreams[i], expected) {
      a := storedStreams[i]
      b := expected
			t.Errorf("Stream at index %d does not match expected content", i)
      t.Errorf("%s ?= %s, %s ?= %s, %s ?= %s, %s ?= %s, %d ?= %d", a.TvgID, b.TvgID, a.Title, b.Title, a.Group, b.Group, a.LogoURL, b.LogoURL, len(a.URLs), len(b.URLs))
      for _, url := range a.URLs {
        t.Errorf("a: %s, %d", url.Content, url.M3UIndex)
      }
      for _, url := range b.URLs {
        t.Errorf("b: %s, %d", url.Content, url.M3UIndex)
      }
      t.FailNow()
		}
	}
}

// streamInfoEqual checks if two StreamInfo objects are equal.
func streamInfoEqual(a, b database.StreamInfo) bool {
	if a.TvgID != b.TvgID || a.Title != b.Title || a.Group != b.Group || a.LogoURL != b.LogoURL || len(a.URLs) != len(b.URLs) {
		return false
	}

	for i, url := range a.URLs {
		if url.Content != b.URLs[i].Content || url.M3UIndex != b.URLs[i].M3UIndex {
			return false
		}
	}

	return true
}
