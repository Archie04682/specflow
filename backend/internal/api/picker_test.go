package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"example.com/specflow/backend/internal/picker"
)

func pickRequest(method, remote, origin string) *http.Request {
	request := httptest.NewRequest(method, "/api/project/pick", nil)
	request.RemoteAddr = remote
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	return request
}

func pickResponse(t *testing.T, request *http.Request) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	recorder := httptest.NewRecorder()
	projectPicker(recorder, request)

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v (%s)", err, recorder.Body.String())
	}
	return recorder, body
}

func withPickFolder(t *testing.T, fn func(context.Context) (string, error)) {
	t.Helper()
	original := pickFolder
	pickFolder = fn
	t.Cleanup(func() { pickFolder = original })
}

func TestProjectPickerReturnsTheChosenPath(t *testing.T) {
	withPickFolder(t, func(context.Context) (string, error) { return "/tmp/project", nil })

	response, body := pickResponse(t, pickRequest(http.MethodPost, "127.0.0.1:5000", ""))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if body["path"] != "/tmp/project" {
		t.Fatalf("path = %v, want /tmp/project", body["path"])
	}
}

func TestProjectPickerReportsCancellationAsANormalOutcome(t *testing.T) {
	withPickFolder(t, func(context.Context) (string, error) { return "", picker.ErrCancelled })

	response, body := pickResponse(t, pickRequest(http.MethodPost, "[::1]:5000", ""))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if body["cancelled"] != true {
		t.Fatalf("body = %v, want cancelled", body)
	}
}

func TestProjectPickerReportsAnUnavailableDialog(t *testing.T) {
	withPickFolder(t, func(context.Context) (string, error) { return "", picker.ErrUnsupported })

	response, _ := pickResponse(t, pickRequest(http.MethodPost, "127.0.0.1:5000", ""))
	if response.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", response.Code)
	}
}

func TestProjectPickerReportsADialogFailure(t *testing.T) {
	withPickFolder(t, func(context.Context) (string, error) { return "", errors.New("dialog crashed") })

	response, body := pickResponse(t, pickRequest(http.MethodPost, "127.0.0.1:5000", ""))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", response.Code)
	}
	if body["error"] != "dialog crashed" {
		t.Fatalf("error = %v", body["error"])
	}
}

func TestProjectPickerRefusesRemoteClients(t *testing.T) {
	called := false
	withPickFolder(t, func(context.Context) (string, error) {
		called = true
		return "/tmp/project", nil
	})

	response, _ := pickResponse(t, pickRequest(http.MethodPost, "192.0.2.1:5000", ""))
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
	if called {
		t.Fatal("the dialog was invoked for a remote client")
	}
}

func TestProjectPickerRefusesForeignOrigins(t *testing.T) {
	withPickFolder(t, func(context.Context) (string, error) { return "/tmp/project", nil })

	response, _ := pickResponse(
		t,
		pickRequest(http.MethodPost, "127.0.0.1:5000", "http://evil.example"),
	)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
}

func TestProjectPickerAcceptsTheViteDevOrigin(t *testing.T) {
	withPickFolder(t, func(context.Context) (string, error) { return "/tmp/project", nil })

	response, _ := pickResponse(
		t,
		pickRequest(http.MethodPost, "127.0.0.1:5000", "http://localhost:5173"),
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
}
