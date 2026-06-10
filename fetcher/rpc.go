package fetcher

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	rpcEndpoint     = "https://solana.publicnode.com"
	rpcFallback     = "https://api.mainnet-beta.solana.com"
	programAddress  = "LBUZKhRxPF3XUpBCjp4YzTKgLccjZhTSDM9YuVaPwxo"
	jupiterPriceURL = "https://api.jup.ag/tokens/v1/price?ids=%s"
	lamportsPerSOL  = 1_000_000_000.0
	pollMaxTxs      = 40
)

// RPCFetcher queries a Solana RPC directly — no Cloudflare, no cookies, no batch dependency.
type RPCFetcher struct {
	rpcURL string
	client *http.Client
	mu     sync.Mutex
}

// NewRPC creates an RPCFetcher. Pass empty string for the default endpoint.
func NewRPC(rpcURL string) *RPCFetcher {
	if rpcURL == "" {
		rpcURL = rpcEndpoint
	}
	return &RPCFetcher{
		rpcURL: rpcURL,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Fetch pulls recent open_position transactions for the LBUZ program.
// Uses individual getTransaction calls with pacing to avoid rate limits.
func (f *RPCFetcher) Fetch() (*Response, error) {
	sigs, err := f.getSignatures(pollMaxTxs)
	if err != nil {
		return nil, fmt.Errorf("get signatures: %w", err)
	}

	resp := &Response{
		Success: true,
		Data: struct {
			Transactions []Transaction `json:"transactions"`
			Cursor       int64         `json:"cursor"`
		}{
			Transactions: make([]Transaction, 0, len(sigs)),
		},
		Metadata: Metadata{
			Tokens: make(map[string]TokenMeta),
		},
	}

	for _, s := range sigs {
		tx, err := f.getTransaction(s.Signature)
		if err != nil {
			// Rate limited? Pause and retry once
			time.Sleep(3 * time.Second)
			tx, err = f.getTransaction(s.Signature)
			if err != nil {
				continue
			}
		}

		bt := int64(0)
		if s.BlockTime != nil {
			bt = *s.BlockTime
		}

		txn := f.parseTransaction(tx, s.Slot, bt)
		if txn != nil {
			resp.Data.Transactions = append(resp.Data.Transactions, *txn)
		} else {
			slog.Debug("parse rejected",
				"sig", s.Signature[:20],
				"slot", s.Slot,
				"has_lbuz", checkHasLBUZ(tx),
				"num_instructions", len(tx.Tx.Message.Instructions),
				"num_accounts", len(tx.Tx.Message.AccountKeys),
				"pre0", tx.Meta.PreBalances[0],
				"post0", tx.Meta.PostBalances[0],
				"fee", tx.Meta.Fee,
			)
		}

		// Pace: 40 calls in 120s = 3s each
		time.Sleep(2 * time.Second)
	}
	// Resolve token metadata in batch
	f.resolveTokenMetadata(resp)
	return resp, nil
}

// checkHasLBUZ checks if a transaction actually invokes the LBUZ program.
// Uses log messages (reliable) and falls back to instruction/programIdIndex check.
func checkHasLBUZ(tx *transactionResult) bool {
	// Primary check: log messages — "Program LBUZ... invoke" means actual invocation
	for _, log := range tx.Meta.LogMessages {
		if strings.HasPrefix(log, "Program "+programAddress+" invoke") {
			return true
		}
	}

	// Fallback: check instruction programIdIndex for direct (non-CPI) calls
	for _, instr := range tx.Tx.Message.Instructions {
		if instr.ProgramIDIndex < len(tx.Tx.Message.AccountKeys) &&
			tx.Tx.Message.AccountKeys[instr.ProgramIDIndex] == programAddress {
			return true
		}
	}
	return false
}

type getSigsResult struct {
	Signature string `json:"signature"`
	Slot      int64  `json:"slot"`
	BlockTime *int64 `json:"blockTime"`
	Err       any    `json:"err"`
}

type transactionResult struct {
	Slot      int64   `json:"slot"`
	BlockTime *int64  `json:"blockTime"`
	Meta      *txMeta `json:"meta"`
	Tx        txBody  `json:"transaction"`
}

type txMeta struct {
	Err               any            `json:"err"`
	Fee               int64          `json:"fee"`
	PreBalances       []int64        `json:"preBalances"`
	PostBalances      []int64        `json:"postBalances"`
	PreTokenBalances  []tokenBalance `json:"preTokenBalances"`
	PostTokenBalances []tokenBalance `json:"postTokenBalances"`
	InnerInstructions []innerInstr   `json:"innerInstructions"`
	LogMessages       []string       `json:"logMessages"`
}

type innerInstr struct {
	Index        int              `json:"index"`
	Instructions []innerInstrItem `json:"instructions"`
}

type innerInstrItem struct {
	ProgramIDIndex int `json:"programIdIndex"`
}

type tokenBalance struct {
	AccountIndex int    `json:"accountIndex"`
	Mint         string `json:"mint"`
	Owner        string `json:"owner"`
	UITokenAmount struct {
		UIAmount *float64 `json:"uiAmount"`
	} `json:"uiTokenAmount"`
}

type txBody struct {
	Message struct {
		AccountKeys []string `json:"accountKeys"`
		Instructions []struct {
			ProgramIDIndex int `json:"programIdIndex"`
		} `json:"instructions"`
	} `json:"message"`
	Signatures []string `json:"signatures"`
}

// --- RPC calls with fallback ---

func (f *RPCFetcher) rpcCall(method string, params any, result any) error {
	url := f.getURL()
	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	}
	b, _ := json.Marshal(body)

	resp, err := f.client.Post(url, "application/json", strings.NewReader(string(b)))
	if err != nil {
		return fmt.Errorf("rpc post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return fmt.Errorf("rate limited (429)")
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read rpc response: %w", err)
	}

	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &rpcResp); err != nil {
		return fmt.Errorf("unmarshal rpc response: %w", err)
	}
	if rpcResp.Error != nil {
		return fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	return json.Unmarshal(rpcResp.Result, result)
}

func (f *RPCFetcher) getURL() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rpcURL
}

func (f *RPCFetcher) fallback() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rpcURL != rpcFallback {
		f.rpcURL = rpcFallback
	}
}

func (f *RPCFetcher) getSignatures(limit int) ([]getSigsResult, error) {
	var result []getSigsResult
	params := []any{
		programAddress,
		map[string]any{"limit": limit},
	}
	if err := f.rpcCall("getSignaturesForAddress", params, &result); err != nil {
		f.fallback()
		return nil, err
	}
	filtered := make([]getSigsResult, 0, len(result))
	for _, s := range result {
		if s.Err == nil {
			filtered = append(filtered, s)
		}
	}
	return filtered, nil
}

func (f *RPCFetcher) getTransaction(sig string) (*transactionResult, error) {
	var result transactionResult
	params := []any{
		sig,
		map[string]any{
			"encoding":                       "json",
			"maxSupportedTransactionVersion": 0,
			"commitment":                     "confirmed",
		},
	}
	if err := f.rpcCall("getTransaction", params, &result); err != nil {
		return nil, err
	}
	if result.Meta == nil || result.Meta.Err != nil {
		return nil, fmt.Errorf("failed tx or missing meta")
	}
	return &result, nil
}

// --- Transaction parsing ---

func (f *RPCFetcher) parseTransaction(tx *transactionResult, slot int64, blockTime int64) *Transaction {
	accounts := tx.Tx.Message.AccountKeys
	signatures := tx.Tx.Signatures
	if len(signatures) == 0 || len(accounts) == 0 {
		return nil
	}

	// Must have an instruction or log indicating LBUZ program invocation
	// For v0 transactions with address lookup tables, instructions reference
	// loaded addresses not in accountKeys, so logs are the reliable signal.
	hasLBUZ := false
	for _, log := range tx.Meta.LogMessages {
		if strings.HasPrefix(log, "Program "+programAddress+" invoke") {
			hasLBUZ = true
			break
		}
	}
	if !hasLBUZ {
		return nil
	}

	feePayer := accounts[0]
	txHash := signatures[0]

	// SOL deposited = position opened (pre > post, minus fee)
	var solValue int64
	fee := tx.Meta.Fee
	for i, pre := range tx.Meta.PreBalances {
		post := tx.Meta.PostBalances[i]
		if i < len(accounts) && accounts[i] == feePayer {
			diff := pre - post - fee
			if diff > 0 {
				solValue = diff
			}
		}
	}

	// Check WSOL balance if no native SOL change
	if solValue <= 0 {
		for _, post := range tx.Meta.PostTokenBalances {
			if post.Owner == feePayer && isSOL(post.Mint) && post.UITokenAmount.UIAmount != nil {
				solValue = int64(*post.UITokenAmount.UIAmount * lamportsPerSOL)
				break
			}
		}
	}

	if solValue <= 0 {
		slog.Debug("parse zero sol",
			"sig", txHash[:16],
			"pre0", tx.Meta.PreBalances[0],
			"post0", tx.Meta.PostBalances[0],
			"fee_payer", feePayer,
			"accts0", accounts[0],
			"len_accts", len(accounts),
			"len_pre", len(tx.Meta.PreBalances),
		)
		return nil
	}

	// Collect non-SOL token addresses
	tokens := make([]string, 0)
	seen := make(map[string]bool)
	for _, post := range tx.Meta.PostTokenBalances {
		if post.Owner == feePayer && !isSOL(post.Mint) && !seen[post.Mint] {
			seen[post.Mint] = true
			tokens = append(tokens, post.Mint)
		}
	}
	for _, pre := range tx.Meta.PreTokenBalances {
		if pre.Owner == feePayer && !isSOL(pre.Mint) && !seen[pre.Mint] {
			seen[pre.Mint] = true
			tokens = append(tokens, pre.Mint)
		}
	}

	return &Transaction{
		BlockTime:   blockTime,
		Slot:        slot,
		TxHash:      txHash,
		Fee:         fee,
		Signer:      []string{feePayer},
		Tags:        []string{"open_position"},
		SolValueRaw: fmt.Sprintf("%d", solValue),
		Tokens:      tokens,
	}
}

func isSOL(mint string) bool {
	return mint == "So11111111111111111111111111111111111111111" ||
		mint == "So11111111111111111111111111111111111111112"
}

// --- Token metadata via Jupiter API ---

func (f *RPCFetcher) resolveTokenMetadata(resp *Response) {
	if len(resp.Data.Transactions) == 0 {
		return
	}

	tokens := make(map[string]bool)
	for _, tx := range resp.Data.Transactions {
		for _, t := range tx.Tokens {
			tokens[t] = true
		}
	}
	if len(tokens) == 0 {
		return
	}

	ids := make([]string, 0, len(tokens))
	for t := range tokens {
		ids = append(ids, t)
	}

	url := fmt.Sprintf(jupiterPriceURL, strings.Join(ids, ","))
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("accept", "application/json")

	r, err := f.client.Do(req)
	if err != nil || r.StatusCode != http.StatusOK {
		if r != nil {
			r.Body.Close()
		}
		return
	}
	defer r.Body.Close()

	raw, _ := io.ReadAll(r.Body)
	var jupResp map[string]struct {
		ID     string  `json:"id"`
		Mint   string  `json:"mint"`
		Name   string  `json:"name"`
		Symbol string  `json:"symbol"`
		Price  float64 `json:"price"`
	}
	if err := json.Unmarshal(raw, &jupResp); err != nil {
		return
	}

	for mint, info := range jupResp {
		resp.Metadata.Tokens[mint] = TokenMeta{
			TokenAddress: info.Mint,
			TokenName:    info.Name,
			TokenSymbol:  info.Symbol,
			PriceUSDT:    info.Price,
		}
	}
}