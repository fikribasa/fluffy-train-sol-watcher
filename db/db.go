package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

type OpenPosition struct {
	TxHash       string
	BlockTime    int64
	Slot         int64
	Wallet       string
	SolValue     float64
	TokenAddress string
	TokenName    string
	TokenSymbol  string
	TokenPrice   float64
	Fee          int64
	CreatedAt    int64
}

const schema = `
CREATE TABLE IF NOT EXISTS open_positions (
	tx_hash       TEXT PRIMARY KEY,
	block_time    INTEGER NOT NULL,
	slot          INTEGER NOT NULL,
	wallet        TEXT NOT NULL,
	sol_value     REAL NOT NULL,
	token_address TEXT NOT NULL,
	token_name    TEXT,
	token_symbol  TEXT,
	token_price   REAL,
	fee           INTEGER NOT NULL,
	created_at    INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_open_positions_block_time ON open_positions(block_time DESC);
CREATE INDEX IF NOT EXISTS idx_open_positions_wallet ON open_positions(wallet);
CREATE INDEX IF NOT EXISTS idx_open_positions_token ON open_positions(token_address);
`

func New(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Single writer is safe; WAL gives better read concurrency
	if _, err := conn.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	if _, err := conn.Exec(schema); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	return &DB{conn: conn}, nil
}

func (d *DB) Close() error {
	return d.conn.Close()
}

// Upsert inserts a position. Silently skips if tx_hash already exists.
// Returns (true, nil) if inserted, (false, nil) if duplicate.
func (d *DB) Upsert(p *OpenPosition) (inserted bool, err error) {
	if p.CreatedAt == 0 {
		p.CreatedAt = time.Now().Unix()
	}

	const q = `
	INSERT OR IGNORE INTO open_positions
		(tx_hash, block_time, slot, wallet, sol_value, token_address, token_name, token_symbol, token_price, fee, created_at)
	VALUES
		(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	res, err := d.conn.Exec(q,
		p.TxHash,
		p.BlockTime,
		p.Slot,
		p.Wallet,
		p.SolValue,
		p.TokenAddress,
		p.TokenName,
		p.TokenSymbol,
		p.TokenPrice,
		p.Fee,
		p.CreatedAt,
	)
	if err != nil {
		return false, fmt.Errorf("upsert open_position %s: %w", p.TxHash, err)
	}

	rows, _ := res.RowsAffected()
	return rows > 0, nil
}

// Count returns total rows — useful for health-check logging.
func (d *DB) Count() (int64, error) {
	var n int64
	err := d.conn.QueryRow(`SELECT COUNT(*) FROM open_positions`).Scan(&n)
	return n, err
}
