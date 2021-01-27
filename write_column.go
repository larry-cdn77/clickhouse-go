package clickhouse

import (
	"database/sql"
	"database/sql/driver"

	"github.com/ClickHouse/clickhouse-go/lib/data"
)

// Interface for Clickhouse driver
type Clickhouse interface {
	Block() (*data.Block, error)
	Prepare(query string) (driver.Stmt, error)
	Begin() (driver.Tx, error)
	Commit() error
	Rollback() error
	Close() error
	WriteBlock(block *data.Block) error
	WriteValue(block *data.Block, column int, v driver.Value) error
}

func OpenDirect(dsn string) (Clickhouse, error) {
	return open(dsn)
}

func (ch *clickhouse) Block() (*data.Block, error) {
	if ch.block == nil {
		return nil, sql.ErrTxDone
	}
	return ch.block, nil
}

func (ch *clickhouse) WriteBlock(block *data.Block) error {
	if block == nil {
		return sql.ErrTxDone
	}
	return ch.writeBlock(block, "")
}

func (ch *clickhouse) WriteValue(block *data.Block, column int, v driver.Value) error {
	return block.AppendColumn(column, v)
}
