package tableengines

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"fmt"
	"strings"

	"github.com/jackc/pgx"

	"github.com/mkabilov/pg2ch/pkg/config"
	"github.com/mkabilov/pg2ch/pkg/message"
	"github.com/mkabilov/pg2ch/pkg/utils"
)

type CollapsingMergeTreeTable struct {
	genericTable

	signColumn string
}

func NewCollapsingMergeTree(conn *sql.DB, name string, tblCfg config.Table) *CollapsingMergeTreeTable {
	t := CollapsingMergeTreeTable{
		genericTable: newGenericTable(conn, name, tblCfg),
		signColumn:   tblCfg.SignColumn,
	}
	t.chColumns = append(t.chColumns, tblCfg.SignColumn)

	t.mergeQueries = []string{fmt.Sprintf("INSERT INTO %[1]s (%[2]s) SELECT %[2]s FROM %[3]s ORDER BY %[4]s",
		t.mainTable, strings.Join(t.chColumns, ", "), t.bufferTable, t.bufferRowIdColumn)}

	go t.backgroundMerge()

	return &t
}

func (t *CollapsingMergeTreeTable) Sync(pgTx *pgx.Tx) error {
	return t.genSync(pgTx, t)
}

func (t *CollapsingMergeTreeTable) Write(p []byte) (n int, err error) {
	rec, err := csv.NewReader(bytes.NewReader(p)).Read()
	if err != nil {
		return 0, err
	}

	row := append(t.convertStrings(rec), 1)
	if t.bufferTable != "" {
		row = append(row, t.bufferRowId)
	}

	if err := t.stmntExec(row); err != nil {
		return 0, fmt.Errorf("could not insert: %v", err)
	}
	t.bufferRowId++

	return len(p), nil
}

func (t *CollapsingMergeTreeTable) Insert(lsn utils.LSN, new message.Row) error {
	return t.processCommandSet(commandSet{
		append(t.convertTuples(new), 1),
	})
}

func (t *CollapsingMergeTreeTable) Update(lsn utils.LSN, old, new message.Row) error {
	return t.processCommandSet(commandSet{
		append(t.convertTuples(old), -1),
		append(t.convertTuples(new), 1),
	})
}

func (t *CollapsingMergeTreeTable) Delete(lsn utils.LSN, old message.Row) error {
	return t.processCommandSet(commandSet{
		append(t.convertTuples(old), -1),
	})
}
