package cifra

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestParsers(t *testing.T) {
	ask, at, err := parseQuote([]byte(`{"q":[{"c":"USDT-RUB","bap":"86.75","ltt":"2026-09-07T10:00:00Z"}]}`), time.Time{})
	if err != nil || ask != 86.75 || at.Hour() != 10 {
		t.Fatalf("quote ask=%v at=%v err=%v", ask, at, err)
	}
	candles, err := parseCandles([]byte(`{"hloc":{"USDT-RUB":[[90,85,89,88],[89,84,88,86]]},"xSeries":{"USDT-RUB":[1788768000,1788771600]}}`))
	if err != nil || len(candles) != 2 || candles[1].Close != 86 {
		t.Fatalf("candles=%v err=%v", candles, err)
	}
}

func TestSignedRequest(t *testing.T) {
	var gotBody, gotSignature, gotPublicKey, gotTimestamp, gotContentType, gotPath string
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		gotSignature = r.Header.Get("X-NtApi-Sig")
		gotPublicKey = r.Header.Get("X-NtApi-PublicKey")
		gotTimestamp = r.Header.Get("X-NtApi-Timestamp")
		gotContentType = r.Header.Get("Content-Type")
		gotPath = r.URL.Path
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
	})}
	c := New("https://example.invalid/api", "public", "secret", httpClient)
	c.Now = func() time.Time { return time.Unix(123456, 0) }
	if _, err := c.signed(t.Context(), "getStockQuotesJson", map[string]any{"tickers": "USDT-RUB"}); err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write([]byte(`{"tickers":"USDT-RUB"}123456`))
	if gotSignature != hex.EncodeToString(mac.Sum(nil)) {
		t.Fatalf("signature mismatch")
	}
	if gotBody != `{"tickers":"USDT-RUB"}` || gotPath != "/api/getStockQuotesJson" || gotContentType != "application/json" {
		t.Fatalf("body=%q path=%q content-type=%q", gotBody, gotPath, gotContentType)
	}
	if gotPublicKey != "public" || gotTimestamp != "123456" {
		t.Fatalf("public-key=%q timestamp=%q", gotPublicKey, gotTimestamp)
	}
}

func TestCloudflareErrorIsSanitized(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 403,
			Body:       io.NopCloser(strings.NewReader(`<html>blocked by Cloudflare and a very long page</html>`)),
			Header:     http.Header{"Content-Type": {"text/html"}},
		}, nil
	})}
	c := New("https://example.invalid/api", "public", "secret", httpClient)
	_, err := c.signed(t.Context(), "getHloc", nil)
	if err == nil || err.Error() != "HTTP 403 (Cloudflare block)" {
		t.Fatalf("err=%v", err)
	}
}

func TestAPIErrorIsReported(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"code":4,"errMsg":"Bad signature"}`)),
			Header:     http.Header{"Content-Type": {"application/json"}},
		}, nil
	})}
	c := New("https://example.invalid/api", "public", "secret", httpClient)
	_, err := c.signed(t.Context(), "getHloc", nil)
	if err == nil || err.Error() != "API code 4: Bad signature" {
		t.Fatalf("err=%v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
