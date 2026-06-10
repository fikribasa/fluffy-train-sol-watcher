package fetcher

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/net/http2"
	utls "github.com/refraction-networking/utls"
)

const solscanURL = "https://api-v2.solscan.io/v2/program/transaction" +
	"?address=LBUZKhRxPF3XUpBCjp4YzTKgLccjZhTSDM9YuVaPwxo" +
	"&page_size=40" +
	"&sort=desc" +
	"&instruction[]=LBUZKhRxPF3XUpBCjp4YzTKgLccjZhTSDM9YuVaPwxodbc0ea47bebf6650" +
	"&hide_spam=true" +
	"&hide_failed=true"

// Response mirrors the Solscan API response structure.
type Response struct {
	Success bool `json:"success"`
	Data    struct {
		Transactions []Transaction `json:"transactions"`
		Cursor       int64         `json:"cursor"`
	} `json:"data"`
	Metadata Metadata `json:"metadata"`
}

type Transaction struct {
	BlockTime         int64    `json:"blockTime"`
	Slot              int64    `json:"slot"`
	TxHash            string   `json:"txHash"`
	Fee               int64    `json:"fee"`
	Status            string   `json:"status"`
	Signer            []string `json:"signer"`
	Tags              []string `json:"tags"`
	SolValueRaw       string   `json:"sol_value"`
	Tokens            []string `json:"tokens"`
	ParsedInstruction []struct {
		Type string `json:"type"`
	} `json:"parsedInstruction"`
}

type Metadata struct {
	Tokens map[string]TokenMeta `json:"tokens"`
}

type TokenMeta struct {
	TokenAddress string  `json:"token_address"`
	TokenName    string  `json:"token_name"`
	TokenSymbol  string  `json:"token_symbol"`
	PriceUSDT    float64 `json:"price_usdt"`
}

// FetcherI is the common interface for fetching transactions.
// Both Fetcher (solscan cookie) and RPCFetcher (Solana RPC) implement it.
type FetcherI interface {
	Fetch() (*Response, error)
}

// Fetcher queries the Solscan API with a cf_clearance cookie.
// Optionally reloads the cookie from a cache file on every call.
type Fetcher struct {
	client     *http.Client
	cookie     string
	cookieFile string
}

// New creates a Fetcher with a static cookie string.
func New(cookie string) *Fetcher {
	return &Fetcher{
		client: &http.Client{
			Timeout:   15 * time.Second,
			Transport: newUTLSTransport(),
		},
		cookie: cookie,
	}
}

// NewWithCookieCache creates a Fetcher that reloads the cookie from cacheFile
// on every Fetch() call. The initial cookie is used as fallback.
func NewWithCookieCache(cookie, cacheFile string) *Fetcher {
	f := New(cookie)
	f.cookieFile = cacheFile
	return f
}

// loadCookie reloads the cookie from the cache file if configured.
func (f *Fetcher) loadCookie() {
	if f.cookieFile == "" {
		return
	}
	data, err := os.ReadFile(f.cookieFile)
	if err != nil {
		return
	}
	c := strings.TrimSpace(string(data))
	if c != "" && c != f.cookie {
		f.cookie = c
	}
}

// newUTLSTransport returns an http2.RoundTripper that mimics Chrome's TLS
// fingerprint (defeating Cloudflare) and speaks HTTP/2.
func newUTLSTransport() http.RoundTripper {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return &http2.Transport{
		AllowHTTP: false,
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			conn, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			tlsConn := utls.UClient(conn, &utls.Config{
				ServerName: strings.Split(addr, ":")[0],
				NextProtos: []string{"h2", "http/1.1"},
			}, utls.HelloChrome_Auto)
			if err := tlsConn.HandshakeContext(ctx); err != nil {
				conn.Close()
				return nil, err
			}
			return tlsConn, nil
		},
	}
}

// Fetch queries the Solscan API. If cookieFile is set, reloads the cookie
// from the cache file before each request.
func (f *Fetcher) Fetch() (*Response, error) {
	f.loadCookie()

	req, err := http.NewRequest(http.MethodGet, solscanURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36")
	req.Header.Set("accept", "application/json, text/plain, */*")
	req.Header.Set("accept-language", "en-AU,en;q=0.8")
	req.Header.Set("dnt", "1")
	req.Header.Set("origin", "https://solscan.io")
	req.Header.Set("referer", "https://solscan.io/")
	req.Header.Set("sec-ch-ua", `"Brave";v="149", "Chromium";v="149", "Not)A;Brand";v="24"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"Windows"`)
	req.Header.Set("sec-fetch-dest", "empty")
	req.Header.Set("sec-fetch-mode", "cors")
	req.Header.Set("sec-fetch-site", "same-site")
	req.Header.Set("sec-gpc", "1")
	req.Header.Set("Cookie", f.cookie)

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, body)
	}

	var result Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("solscan returned success=false")
	}

	return &result, nil
}