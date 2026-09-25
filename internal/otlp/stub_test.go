package otlp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStub(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/traces", nil)
	rr := httptest.NewRecorder()
	Handler(rr, req)
	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("status %d", rr.Code)
	}
}
