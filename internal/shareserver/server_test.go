package shareserver

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T) (*httptest.Server, *Server) {
	t.Helper()
	store, err := NewFSStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv, err := NewServer(Config{
		BearerToken: "test-token",
		PublicURL:   "https://example.test",
	}, store)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, srv
}

func upload(t *testing.T, ts *httptest.Server, body any, token string) *http.Response {
	t.Helper()
	buf, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/shares", bytes.NewReader(buf))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestUploadRoundTripAndView(t *testing.T) {
	ts, _ := newTestServer(t)
	body := uploadRequest{
		Name:            "test share",
		ExpiresInSecond: int64((1 * time.Hour).Seconds()),
		Session: ShareSession{
			ShortID: "abc", Project: "~/app", Source: "claude",
			FirstPrompt: "hello", MessageCount: 2, StartedAt: time.Now().UTC(),
		},
		Messages: []ShareMsg{
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "hello back"},
		},
	}
	resp := upload(t, ts, body, "test-token")
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 201, got %d: %s", resp.StatusCode, b)
	}
	var ur uploadResponse
	_ = json.NewDecoder(resp.Body).Decode(&ur)
	resp.Body.Close()
	if ur.ID == "" || ur.URL == "" {
		t.Fatalf("empty id/url in response: %+v", ur)
	}
	if !strings.HasPrefix(ur.URL, "https://example.test/s/") {
		t.Fatalf("unexpected URL: %s", ur.URL)
	}

	// HTML view
	hResp, err := ts.Client().Get(ts.URL + "/s/" + ur.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer hResp.Body.Close()
	if hResp.StatusCode != http.StatusOK {
		t.Fatalf("html view status = %d", hResp.StatusCode)
	}
	html, _ := io.ReadAll(hResp.Body)
	if !bytes.Contains(html, []byte("hello back")) {
		t.Errorf("expected message body in HTML")
	}
	if !bytes.Contains(html, []byte("test share")) {
		t.Errorf("expected name in HTML")
	}

	// Raw download — explicit Accept-Encoding keeps Go's http client from
	// transparently decompressing so we can verify the bytes are gzipped.
	rReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/s/"+ur.ID+"/raw.json.gz", nil)
	rReq.Header.Set("Accept-Encoding", "gzip")
	rResp, _ := ts.Client().Do(rReq)
	defer rResp.Body.Close()
	if rResp.StatusCode != http.StatusOK {
		t.Fatalf("raw status = %d", rResp.StatusCode)
	}
	if rResp.Header.Get("Content-Encoding") != "gzip" {
		t.Errorf("raw missing gzip encoding")
	}
	raw, _ := io.ReadAll(rResp.Body)
	if len(raw) < 2 || raw[0] != 0x1f || raw[1] != 0x8b {
		t.Errorf("raw body is not gzip magic: %x", raw[:min(4, len(raw))])
	}
}

func TestUploadRejectsBadAuth(t *testing.T) {
	ts, _ := newTestServer(t)
	body := uploadRequest{
		ExpiresInSecond: int64((1 * time.Hour).Seconds()),
		Messages:        []ShareMsg{{Role: "user", Content: "hi"}},
	}
	resp := upload(t, ts, body, "wrong")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestUploadRejectsTooShortLifetime(t *testing.T) {
	ts, _ := newTestServer(t)
	body := uploadRequest{
		ExpiresInSecond: 10, // way below min
		Messages:        []ShareMsg{{Role: "user", Content: "hi"}},
	}
	resp := upload(t, ts, body, "test-token")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestUploadRejectsEmptyMessages(t *testing.T) {
	ts, _ := newTestServer(t)
	body := uploadRequest{
		ExpiresInSecond: int64((1 * time.Hour).Seconds()),
		Messages:        nil,
	}
	resp := upload(t, ts, body, "test-token")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestViewReturnsGoneForExpired(t *testing.T) {
	ts, srv := newTestServer(t)
	// Bypass the server's min-lifetime by putting directly through store.
	id, _ := NewID()
	gz, _ := gzipJSON(Share{ID: id, Messages: []ShareMsg{{Role: "user", Content: "x"}}})
	_ = srv.store.Put(nil, id, time.Now().Add(-time.Minute), gz)

	resp, _ := ts.Client().Get(ts.URL + "/s/" + id)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusGone {
		t.Fatalf("want 410, got %d", resp.StatusCode)
	}
}

func TestDeleteRemovesShare(t *testing.T) {
	ts, _ := newTestServer(t)
	body := uploadRequest{
		ExpiresInSecond: int64((1 * time.Hour).Seconds()),
		Session:         ShareSession{ShortID: "abc", StartedAt: time.Now()},
		Messages:        []ShareMsg{{Role: "user", Content: "hi"}},
	}
	resp := upload(t, ts, body, "test-token")
	var ur uploadResponse
	_ = json.NewDecoder(resp.Body).Decode(&ur)
	resp.Body.Close()

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/v1/shares/"+ur.ID, nil)
	req.Header.Set("Authorization", "Bearer test-token")
	delResp, _ := ts.Client().Do(req)
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("want 204, got %d", delResp.StatusCode)
	}
	// Now GET should 404.
	vResp, _ := ts.Client().Get(ts.URL + "/s/" + ur.ID)
	vResp.Body.Close()
	if vResp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404 after delete, got %d", vResp.StatusCode)
	}
}

func TestConsumeSingleUseDeletes(t *testing.T) {
	ts, _ := newTestServer(t)
	body := uploadRequest{
		Kind:            KindHandoff,
		ExpiresInSecond: int64((1 * time.Hour).Seconds()),
		SingleUse:       true,
		HandoffNote:     "pick up the auth branch",
		Session:         ShareSession{ShortID: "abc", StartedAt: time.Now()},
		Messages:        []ShareMsg{{Role: "user", Content: "hi"}},
	}
	resp := upload(t, ts, body, "test-token")
	var ur uploadResponse
	_ = json.NewDecoder(resp.Body).Decode(&ur)
	resp.Body.Close()

	// First consume returns the share.
	cResp, _ := ts.Client().Post(ts.URL+"/v1/shares/"+ur.ID+"/consume", "", nil)
	if cResp.StatusCode != http.StatusOK {
		t.Fatalf("consume status = %d", cResp.StatusCode)
	}
	var got Share
	_ = json.NewDecoder(cResp.Body).Decode(&got)
	cResp.Body.Close()
	if got.HandoffNote != "pick up the auth branch" {
		t.Errorf("handoff note missing: %+v", got)
	}
	if got.Kind != KindHandoff {
		t.Errorf("kind = %q, want handoff", got.Kind)
	}

	// Second consume returns 404 because the share was deleted.
	cResp2, _ := ts.Client().Post(ts.URL+"/v1/shares/"+ur.ID+"/consume", "", nil)
	defer cResp2.Body.Close()
	if cResp2.StatusCode != http.StatusNotFound {
		t.Fatalf("second consume status = %d, want 404", cResp2.StatusCode)
	}
}

func TestConsumeNonSingleUseKeeps(t *testing.T) {
	ts, _ := newTestServer(t)
	body := uploadRequest{
		Kind:            KindSnapshot,
		ExpiresInSecond: int64((1 * time.Hour).Seconds()),
		Session:         ShareSession{ShortID: "abc", StartedAt: time.Now()},
		Messages:        []ShareMsg{{Role: "user", Content: "hi"}},
	}
	resp := upload(t, ts, body, "test-token")
	var ur uploadResponse
	_ = json.NewDecoder(resp.Body).Decode(&ur)
	resp.Body.Close()

	for i := 0; i < 2; i++ {
		c, _ := ts.Client().Post(ts.URL+"/v1/shares/"+ur.ID+"/consume", "", nil)
		c.Body.Close()
		if c.StatusCode != http.StatusOK {
			t.Fatalf("consume #%d: status = %d", i, c.StatusCode)
		}
	}
}

func TestHandoffHTMLDoesNotLeakTranscript(t *testing.T) {
	ts, _ := newTestServer(t)
	secret := "SECRET-TRANSCRIPT-CONTENTS-SHOULD-NOT-APPEAR-IN-HTML"
	body := uploadRequest{
		Kind:            KindHandoff,
		ExpiresInSecond: int64((1 * time.Hour).Seconds()),
		SingleUse:       true,
		HandoffNote:     "continue the refactor",
		Files: []ShareFile{
			{Path: "~/app/src/auth.go", Action: "edit"},
		},
		Session:  ShareSession{ShortID: "abc", StartedAt: time.Now()},
		Messages: []ShareMsg{{Role: "user", Content: secret}},
	}
	resp := upload(t, ts, body, "test-token")
	var ur uploadResponse
	_ = json.NewDecoder(resp.Body).Decode(&ur)
	resp.Body.Close()

	// HTML view
	hResp, _ := ts.Client().Get(ts.URL + "/s/" + ur.ID)
	html, _ := io.ReadAll(hResp.Body)
	hResp.Body.Close()
	if bytes.Contains(html, []byte(secret)) {
		t.Fatalf("handoff HTML leaked transcript content")
	}
	if !bytes.Contains(html, []byte("continue the refactor")) {
		t.Errorf("handoff note missing from HTML")
	}
	if !bytes.Contains(html, []byte("ses resume --from")) {
		t.Errorf("claim command missing from HTML")
	}
	if !bytes.Contains(html, []byte("single-use")) {
		t.Errorf("single-use disclosure missing from HTML")
	}
	if !bytes.Contains(html, []byte("~/app/src/auth.go")) {
		t.Errorf("files-touched list missing from HTML")
	}

	// Raw download is blocked for handoff
	rResp, _ := ts.Client().Get(ts.URL + "/s/" + ur.ID + "/raw.json.gz")
	defer rResp.Body.Close()
	if rResp.StatusCode != http.StatusForbidden {
		t.Fatalf("raw.json.gz for handoff should be 403, got %d", rResp.StatusCode)
	}
}

func TestHealthz(t *testing.T) {
	ts, _ := newTestServer(t)
	resp, _ := ts.Client().Get(ts.URL + "/healthz")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}
