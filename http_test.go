package pairing_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pairing"
)

func postJSON(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/pairings/next-round", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestHTTPSuccess(t *testing.T) {
	h := pairing.NewHandler()
	rr := postJSON(t, h, `{
		"players": [
			{"id":"A","score":4},{"id":"B","score":4},
			{"id":"C","score":0},{"id":"D","score":0}
		],
		"games": {
			"A":[{"round":1,"opponentId":"C","color":"F"}],
			"C":[{"round":1,"opponentId":"A","color":"S"}],
			"B":[{"round":1,"opponentId":"D","color":"F"}],
			"D":[{"round":1,"opponentId":"B","color":"S"}]
		},
		"byes": {}
	}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	var plan pairing.Plan
	if err := json.Unmarshal(rr.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Round != 2 || len(plan.Pairs) != 2 {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

func TestHTTPHighScoreSumDoesNotOverflow(t *testing.T) {
	h := pairing.NewHandler()
	rr := postJSON(t, h, `{
		"players": [
			{"id":"A","score":9223372036854775807},{"id":"B","score":0},
			{"id":"C","score":9223372036854775807},{"id":"D","score":0}
		]
	}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	var plan pairing.Plan
	if err := json.Unmarshal(rr.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	want := []pairing.Pair{{FirstID: "A", SecondID: "C"}, {FirstID: "B", SecondID: "D"}}
	if len(plan.Pairs) != 2 || plan.Pairs[0] != want[0] || plan.Pairs[1] != want[1] {
		t.Fatalf("pairs = %v, want %v", plan.Pairs, want)
	}
}

func TestHTTPNoPairing(t *testing.T) {
	h := pairing.NewHandler()
	body := bytes.NewBufferString(`{
		"players": [{"id":"A"},{"id":"B"},{"id":"C"},{"id":"D"},{"id":"E"}],
		"byes": {"A":[1],"B":[1],"C":[1],"D":[1],"E":[1]}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/pairings/next-round", body)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "NO_PAIRING") {
		t.Fatalf("body = %s", rr.Body.String())
	}
}

func TestHTTPInvalidHistory(t *testing.T) {
	h := pairing.NewHandler()
	rr := postJSON(t, h, `{
		"players": [{"id":"A"},{"id":"B"}],
		"games": {},
		"byes": {}
	}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "INVALID_HISTORY") {
		t.Fatalf("body = %s", rr.Body.String())
	}
}

func TestHTTPColorDisagreementRejected(t *testing.T) {
	h := pairing.NewHandler()
	rr := postJSON(t, h, `{
		"players": [{"id":"A"},{"id":"B"},{"id":"C"},{"id":"D"}],
		"games": {
			"A":[{"round":1,"opponentId":"B","color":"F"}],
			"B":[{"round":1,"opponentId":"A","color":"F"}]
		}
	}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
}

func TestHTTPBadJSON(t *testing.T) {
	h := pairing.NewHandler()
	rr := postJSON(t, h, `{not json`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
}

func TestHTTPMethodNotAllowed(t *testing.T) {
	h := pairing.NewHandler()
	req := httptest.NewRequest(http.MethodGet, "/pairings/next-round", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", rr.Code)
	}
}
