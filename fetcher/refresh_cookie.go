package fetcher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// solscanChallengeURL is the domain we solve the Cloudflare challenge against.
// We deliberately use the front-page domain (not api-v2) because FlareSolverr
// reliably solves it, and the issued cf_clearance is valid for *.solscan.io.
const solscanChallengeURL = "https://solscan.io/"

// flareSolverrTimeout is the maxTimeout (ms) handed to FlareSolverr.
const flareSolverrTimeout = 120000

// fsRequest is the FlareSolverr v1 request payload.
type fsRequest struct {
	Cmd        string `json:"cmd"`
	URL        string `json:"url"`
	MaxTimeout int    `json:"maxTimeout"`
}

// fsResponse is the FlareSolverr v1 response payload.
type fsResponse struct {
	Status   string `json:"status"`
	Message  string `json:"message"`
	Solution struct {
		URL       string `json:"url"`
		Status    int    `json:"status"`
		UserAgent string `json:"userAgent"`
		Cookies   []struct {
			Name   string  `json:"name"`
			Value  string  `json:"value"`
			Domain string  `json:"domain"`
			Expiry float64 `json:"expiry"`
		} `json:"cookies"`
	} `json:"solution"`
}

// RefreshCookie solves the Cloudflare challenge for solscan.io via FlareSolverr
// and updates the Fetcher's cookie, bound User-Agent, and expiry.
//
// The cf_clearance cookie is bound to the exact User-Agent that solved the
// challenge, so we MUST propagate solution.userAgent to subsequent requests —
// otherwise Cloudflare rejects the cookie with a 403.
func (f *Fetcher) RefreshCookie() error {
	if f.flareSolverrURL == "" {
		return fmt.Errorf("no flaresolverr url configured")
	}

	reqBody, err := json.Marshal(fsRequest{
		Cmd:        "request.get",
		URL:        solscanChallengeURL,
		MaxTimeout: flareSolverrTimeout,
	})
	if err != nil {
		return fmt.Errorf("marshal flaresolverr request: %w", err)
	}

	// FlareSolverr can take a while to solve; allow more than its maxTimeout.
	client := &http.Client{Timeout: time.Duration(flareSolverrTimeout)*time.Millisecond + 30*time.Second}

	resp, err := client.Post(f.flareSolverrURL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("post to flaresolverr: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read flaresolverr response: %w", err)
	}

	var fs fsResponse
	if err := json.Unmarshal(raw, &fs); err != nil {
		return fmt.Errorf("decode flaresolverr response: %w", err)
	}

	if fs.Status != "ok" {
		return fmt.Errorf("flaresolverr status %q: %s", fs.Status, fs.Message)
	}

	var cfValue string
	var cfExpiry float64
	for _, c := range fs.Solution.Cookies {
		if c.Name == "cf_clearance" {
			cfValue = c.Value
			cfExpiry = c.Expiry
			break
		}
	}

	if cfValue == "" {
		return fmt.Errorf("no cf_clearance cookie in flaresolverr solution")
	}

	f.cookie = "cf_clearance=" + cfValue
	if fs.Solution.UserAgent != "" {
		f.userAgent = fs.Solution.UserAgent
	}
	if cfExpiry > 0 {
		f.expiry = time.Unix(int64(cfExpiry), 0)
	}

	slog.Info("cf_clearance refreshed",
		"user_agent", f.userAgent,
		"expiry", f.expiry.Format(time.RFC3339),
	)
	return nil
}
