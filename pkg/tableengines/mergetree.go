package tableengines

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx"

	"github.com/mkabilov/pg2ch/pkg/config"
	"github.com/mkabilov/pg2ch/pkg/message"
	"github.com/mkabilov/pg2ch/pkg/utils"
)

type mergeTreeTable struct {
	genericTable
}

// NewMergeTree instantiates mergeTreeTable
func NewMergeTree(ctx context.Context, connUrl string, tblCfg config.Table, genID *uint64) *mergeTreeTable {
	t := mergeTreeTable{
		genericTable: newGenericTable(ctx, connUrl, tblCfg, genID),
	}

	if t.cfg.ChBufferTable.IsEmpty() {
		return &t
	}

	t.flushQueries = []string{fmt.Sprintf("INSERT INTO %[1]s (%[2]s) SELECT %[2]s FROM %[3]s ORDER BY %[4]s",
		t.cfg.ChMainTable, strings.Join(t.chUsedColumns, ", "), t.cfg.ChBufferTable, t.cfg.BufferTableRowIdColumn)}

	return &t
}

// Sync performs initial sync of the data; pgTx is a transaction in which temporary replication slot is created
func (t *mergeTreeTable) Sync(pgTx *pgx.Tx, lsn utils.LSN) error {
	return t.genSync(pgTx, lsn, t)
}

// Write implements io.Writer which is used during the Sync process, see genSync method
func (t *mergeTreeTable) Write(p []byte) (int, error) {
	if err := t.genSyncWrite(p); err != nil {
		return 0, err
	}

	if t.cfg.GenerationColumn != "" {
		if err := t.bulkUploader.Write([]byte("\t0")); err != nil { // generation id
			return 0, err
		}
	}
	if err := t.bulkUploader.Write([]byte("\n")); err != nil {
		return 0, err
	}

	t.printSyncProgress()

	return len(p), nil
}

// Insert handles incoming insert DML operation
func (t *mergeTreeTable) Insert(lsn utils.LSN, new message.Row) (bool, error) {
	return t.processChTuples(lsn, chTuples{t.convertRow(new)})
}

// Update handles incoming update DML operation
func (t *mergeTreeTable) Update(lsn utils.LSN, old, new message.Row) (bool, error) {
	return t.processChTuples(0, nil)
}

// Delete handles incoming delete DML operation
func (t *mergeTreeTable) Delete(lsn utils.LSN, old message.Row) (bool, error) {
	return t.processChTuples(0, nil)
}
