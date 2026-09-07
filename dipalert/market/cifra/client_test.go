package cifra

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
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
	var gotForm url.Values
	var gotSignature string
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		gotForm, _ = url.ParseQuery(string(body))
		gotSignature = r.Header.Get("X-NtApi-Sig")
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
	})}
	c := New("https://example.invalid", "public", "secret", httpClient)
	c.Now = func() time.Time { return time.UnixMilli(123456) }
	if _, err := c.signed(t.Context(), "getStockQuotesJson", url.Values{"tickers[0]": {"USDT-RUB"}}); err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write([]byte(gotForm.Encode()))
	if gotSignature != hex.EncodeToString(mac.Sum(nil)) {
		t.Fatalf("signature mismatch")
	}
	if !strings.HasSuffix(gotForm.Get("nonce"), "0") || gotForm.Get("apiKey") != "public" {
		t.Fatalf("form=%v", gotForm)
	}
	if gotForm.Get("tickers[0]") != "USDT-RUB" {
		t.Fatalf("form=%v", gotForm)
	}
}

func TestPublicHlocRequest(t *testing.T) {
	var gotPath, gotContentType string
	var gotForm url.Values
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		gotForm, _ = url.ParseQuery(string(body))
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
	})}
	c := New("https://example.invalid/api", "public", "secret", httpClient)
	if _, err := c.public(t.Context(), "get-hloc", url.Values{"id": {"USDT-RUB"}, "timeframe": {"60"}}); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/get-hloc" || gotContentType != "application/x-www-form-urlencoded" {
		t.Fatalf("path=%q content-type=%q", gotPath, gotContentType)
	}
	if gotForm.Get("id") != "USDT-RUB" || gotForm.Get("timeframe") != "60" {
		t.Fatalf("form=%v", gotForm)
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
	_, err := c.public(t.Context(), "get-hloc", nil)
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
	_, err := c.public(t.Context(), "get-hloc", nil)
	if err == nil || err.Error() != "API code 4: Bad signature" {
		t.Fatalf("err=%v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
