package webui

import (
	"strings"
	"testing"
)

// TestUploadShowsProgressAndReportsFailure pins issue #157.
//
// The upload was a `fetch` whose only failure handler was `console.error`: a large
// SFTP write could run for minutes with the file list looking exactly as it did
// before, and a failure looked the same as a slow success — no percentage, no
// "uploading" state, no message, and a button that stayed live so a second click
// started a second POST at the same path.
//
// The three things that fix it are asserted here as source facts, because they are
// what the browser actually runs and no Go-level test can observe a transfer's
// progress events:
//
//  1. the request is an XHR (fetch's request-body stream is not portable), so
//     `upload.onprogress` exists at all;
//  2. the button carries the transfer state and is disabled while it runs;
//  3. a non-2xx answer and a transport error both reach a toast.
func TestUploadShowsProgressAndReportsFailure(t *testing.T) {
	js := readAssetLF(t, "static/js/terminal-panels.js")
	body := between(t, js, "if (win.querySelector('.shell-file-upload-btn')", "\n  /* Exposed so reapplyWindowLanguage")

	if body == "" {
		t.Fatal("the upload handler moved; this test pins the block that sends the file")
	}
	if !strings.Contains(body, "new XMLHttpRequest()") {
		t.Error("the upload must use XHR: fetch has no upload progress event")
	}
	if !strings.Contains(body, "xhr.upload.onprogress") {
		t.Error("no progress handler: the button cannot show how far the transfer is")
	}
	if !strings.Contains(body, "uploadBtn.disabled = true") {
		t.Error("the button must be disabled while a transfer runs, or a second click writes the same path twice")
	}
	if !strings.Contains(body, "uploadState(null)") {
		t.Error("the transfer state must be cleared on every exit path")
	}
	for _, want := range []string{"xhr.onerror", "xhr.onabort", "toast.upload.failed"} {
		if !strings.Contains(body, want) {
			t.Errorf("a failed upload must reach the user through a toast: %s is missing", want)
		}
	}
	// The old silent path, which is the bug itself.
	if strings.Contains(body, "console.error") {
		t.Error("a failure that only reaches the console is the silent failure #157 reports")
	}
}

// TestUploadFailureTextSurvivesANonJSONBody is the companion to the toast: the
// endpoint answers errors as JSON, but a proxy or a dropped connection answers with
// HTML or nothing, and a toast reading "undefined" is its own kind of silence.
func TestUploadFailureTextSurvivesANonJSONBody(t *testing.T) {
	js := readAssetLF(t, "static/js/util.js")
	body := between(t, js, "function uploadErrorText(", "\n}\n")
	if body == "" {
		t.Fatal("uploadErrorText is gone; the upload toast has no message source")
	}
	if !strings.Contains(body, "JSON.parse") {
		t.Error("the error body must be parsed for the endpoint's {\"error\":...} shape")
	}
	if !strings.Contains(body, "catch") {
		t.Error("a non-JSON body must fall back to the status instead of throwing")
	}
	if !strings.Contains(body, "xhr.status") {
		t.Error("the fallback must name the status, or a failure with no body says nothing")
	}
}

// TestUploadStringsExistInEveryLocale keeps the new keys honest: the i18n catalog is
// the only place a UI string may come from, and a key missing from one language falls
// back to the key's own name in that language.
func TestUploadStringsExistInEveryLocale(t *testing.T) {
	cat := loadCatalogs(t)
	for _, key := range []string{"file.upload.progress", "file.upload.network", "file.upload.aborted", "toast.upload.failed"} {
		for _, lang := range []string{"en", "zh-Hans", "zh-Hant"} {
			v, ok := cat[lang][key]
			if !ok {
				t.Errorf("%s: %q is missing", lang, key)
				continue
			}
			if v == "" || v == key {
				t.Errorf("%s: %q is not translated (%q)", lang, key, v)
			}
		}
	}
}
