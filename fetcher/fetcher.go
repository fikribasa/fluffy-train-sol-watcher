package fetcher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const solscanURL = "https://api-v2.solscan.io/v2/program/transaction" +
	"?address=LBUZKhRxPF3XUpBCjp4YzTKgLccjZhTSDM9YuVaPwxo" +
	"&page_size=40" +
	"&sort=desc" +
	"&instruction[]=LBUZKhRxPF3XUpBCjp4YzTKgLccjZhTSDM9YuVaPwxodbc0ea47bebf6650" +
	"&hide_spam=true" +
	"&hide_failed=true"

// FlareSolverr request/response structs

type fsRequest struct {
	CMD            string `json:"cmd"`
	URL            string `json:"url"`
	MaxTimeout     int    `json:"maxTimeout"`
}

type fsResponse struct {
	Status   string     `json:"status"`
	Message  string     `json:"message"`
	Solution fsSolution `json:"solution"`
}

type fsSolution struct {
	URL        string      `json:"url"`
	Status     int         `json:"status"`
	Response   string      `json:"response"` // raw body from target URL
	Cookies    []fsCookie  `json:"cookies"`
}

type fsCookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Solscan response structs

type Response struct {
	Success bool `json:"success"`
	Data    struct {
		Transactions []Transaction `json:"transactions"`
		Cursor       int64         `json:"cursor"`
	} `json:"data"`
	Metadata Metadata `json:"metadata"`
}

type Transaction struct {
	BlockTime   int64    `json:"blockTime"`
	Slot        int64    `json:"slot"`
	TxHash      string   `json:"txHash"`
	Fee         int64    `json:"fee"`
	Status      string   `json:"status"`
	Signer      []string `json:"signer"`
	Tags        []string `json:"tags"`
	SolValueRaw string   `json:"sol_value"`
	Tokens      []string `json:"tokens"`
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

type Fetcher struct {
	client          *http.Client
	flareSolverrURL string
	timeout         int // milliseconds passed to FlareSolverr
}

func New(flareSolverrURL string, timeoutMS int) *Fetcher {
	// HTTP client timeout is slightly longer than FlareSolverr's own timeout
	httpTimeout := time.Duration(timeoutMS+10000) * time.Millisecond
	return &Fetcher{
		client:          &http.Client{Timeout: httpTimeout},
		flareSolverrURL: flareSolverrURL,
		timeout:         timeoutMS,
	}
}

func (f *Fetcher) Fetch() (*Response, error) {
	// Build FlareSolverr request
	fsReq := fsRequest{
		CMD:        "request.get",
		URL:        solscanURL,
		MaxTimeout: f.timeout,
	}

	body, err := json.Marshal(fsReq)
	if err != nil {
		return nil, fmt.Errorf("marshal flaresolverr request: %w", err)
	}

	resp, err := f.client.Post(f.flareSolverrURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("post to flaresolverr: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read flaresolverr response: %w", err)
	}

	var fsResp fsResponse
	if err := json.Unmarshal(rawBody, &fsResp); err != nil {
		return nil, fmt.Errorf("decode flaresolverr response: %w", err)
	}

	if fsResp.Status != "ok" {
		return nil, fmt.Errorf("flaresolverr status %q: %s", fsResp.Status, fsResp.Message)
	}

	if fsResp.Solution.Status != http.StatusOK {
		return nil, fmt.Errorf("solscan returned HTTP %d via flaresolverr", fsResp.Solution.Status)
	}

	// solution.response is the raw Solscan JSON body as a string
	var result Response
	if err := json.Unmarshal([]byte(fsResp.Solution.Response), &result); err != nil {
		return nil, fmt.Errorf("decode solscan response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("solscan returned success=false")
	}

	return &result, nil
}