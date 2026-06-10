package processor

import (
	"strconv"

	"solscan-watcher/db"
	"solscan-watcher/fetcher"
)

const lamportsPerSOL = 1_000_000_000.0

// baseTokens are the well-known quote/base tokens to exclude when finding
// the "interesting" token in a DLMM pair.
var baseTokens = map[string]bool{
	"So11111111111111111111111111111111111111111":  true, // SOL
	"So11111111111111111111111111111111111111112":  true, // WSOL
	"EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v": true, // USDC
}

// Process filters and transforms raw API transactions into OpenPosition records.
// Only returns transactions that:
//   - have the "open_position" tag
//   - sol_value > threshold (in SOL)
func Process(resp *fetcher.Response, thresholdSOL float64) []*db.OpenPosition {
	results := make([]*db.OpenPosition, 0)

	for i := range resp.Data.Transactions {
		tx := &resp.Data.Transactions[i]

		if !hasTag(tx.Tags, "open_position") {
			continue
		}

		solValue, err := parseSolValue(tx.SolValueRaw)
		if err != nil || solValue <= thresholdSOL {
			continue
		}

		if len(tx.Signer) == 0 {
			continue
		}

		tokenAddr := extractToken(tx.Tokens)
		tokenMeta := resolveTokenMeta(tokenAddr, resp.Metadata)

		pos := &db.OpenPosition{
			TxHash:       tx.TxHash,
			BlockTime:    tx.BlockTime,
			Slot:         tx.Slot,
			Wallet:       tx.Signer[0],
			SolValue:     solValue,
			TokenAddress: tokenAddr,
			TokenName:    tokenMeta.TokenName,
			TokenSymbol:  tokenMeta.TokenSymbol,
			TokenPrice:   tokenMeta.PriceUSDT,
			Fee:          tx.Fee,
		}

		results = append(results, pos)
	}

	return results
}

// hasTag checks if a tag exists in the tags slice.
func hasTag(tags []string, target string) bool {
	for _, t := range tags {
		if t == target {
			return true
		}
	}
	return false
}

// parseSolValue converts the raw lamport string to a float64 SOL value.
func parseSolValue(raw string) (float64, error) {
	lamports, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, err
	}
	return float64(lamports) / lamportsPerSOL, nil
}

// extractToken returns the first token address that is not a known base token.
// Returns empty string if all tokens are base tokens.
func extractToken(tokens []string) string {
	for _, addr := range tokens {
		if !baseTokens[addr] {
			return addr
		}
	}
	return ""
}

// resolveTokenMeta looks up token metadata from the response metadata block.
// Returns zero-value TokenMeta if address is empty or not found.
func resolveTokenMeta(address string, meta fetcher.Metadata) fetcher.TokenMeta {
	if address == "" {
		return fetcher.TokenMeta{}
	}
	if m, ok := meta.Tokens[address]; ok {
		return m
	}
	return fetcher.TokenMeta{TokenAddress: address}
}
