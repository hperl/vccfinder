package main

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"reflect"
	"strings"

	log "github.com/Sirupsen/logrus"

	"github.com/coopernurse/gorp"
	"github.com/lib/pq"
)

var (
	DB         *gorp.DbMap
	ErrBadConn = driver.ErrBadConn
	dbname     = "github"
)

type DbObj interface {
	GetId() int64
}

func InitDb() (err error) {
	conn, err := newDBConnection()
	if err != nil {
		return err
	}
	DB = &gorp.DbMap{Db: conn, Dialect: gorp.PostgresDialect{}}
	DB.AddTableWithNameAndSchema(Commit{}, "unstable", "commits").SetKeys(true, "id")
	DB.AddTableWithName(Repository{}, "repositories").SetKeys(true, "id")
	return nil
}

func newDBConnection() (conn *sql.DB, err error) {
	conn, err = sql.Open(
		"postgres",
		"postgres://"+os.Getenv("POSTGRES_CONNECTION")+"/"+dbname,
	)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(5)
	return
}

func ReopenDB() (err error) {
	if DB.Db != nil {
		if e := DB.Db.Close(); e != nil {
			log.Errorf("Failed to close old connection: %v", e)
		}
	}
	return InitDb()
}

func sqlColumns(t reflect.Type) (cols []string) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		col := field.Tag.Get("db")
		if col != "" && col != "-" {
			cols = append(cols, col)
		}
	}
	return
}

func PersistColumn(obj DbObj, col string, val interface{}) error {
	q := PersistColumnSql(obj, col, val)
	_, err := DB.Db.Exec(q, val)
	return err
}
func PersistColumnSql(obj DbObj, col string, val interface{}) string {
	tableName, err := tableName(obj)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("UPDATE %s SET %s = $1 WHERE id = %d", tableName, col, obj.GetId())
}

func PersistColumns(obj DbObj, cols ...string) (err error) {
	q, vals, err := PersistColumnsSql(obj, cols...)
	if err != nil {
		return
	}
	_, err = DB.Db.Exec(q, vals...)

	return
}

func PersistColumnsSql(obj DbObj, cols ...string) (q string, vals []interface{}, err error) {
	var (
		qs []string
		t  = reflect.TypeOf(obj).Elem()
		v  = reflect.ValueOf(obj).Elem()
	)

	tableName, err := tableName(obj)
	if err != nil {
		return
	}

	q = fmt.Sprintf("UPDATE %s SET ", tableName)

	for qField, col := range cols {
		// build query
		f, ok := t.FieldByName(col)
		if !ok {
			return "", nil, fmt.Errorf("field %s not found in commit struct", col)
		}
		sqlField := f.Tag.Get("db")
		if sqlField == "-" || sqlField == "" {
			return "", nil, fmt.Errorf("field %s does not have db tag", col)
		}
		qs = append(qs, fmt.Sprintf("%s = $%d", sqlField, qField+1))

		// build values
		vals = append(vals, v.FieldByName(col).Interface())
	}
	q += strings.Join(qs, ", ")
	q += fmt.Sprintf(" WHERE id = %d", obj.GetId())

	//log.Debugf("%s: %s %v", obj, q, vals)

	return
}

func PersistToolResults(c *Commit) (err error) {
	txn, err := DB.Db.Begin()
	if err != nil {
		return
	}
	// clear old results
	_, err = txn.Exec("DELETE FROM unstable.tool_results WHERE commit_id = $1", c.Id)
	if err != nil {
		return fmt.Errorf("%v: deleting old tool results falied: %v", c, err)
	}

	// prepare insert
	stmt, err := txn.Prepare(pq.CopyInSchema("unstable", "tool_results", "commit_id", "file_name", "line", "reason", "found_by"))
	if err != nil {
		return
	}

	for _, r := range c.ToolResults {
		_, err = stmt.Exec(
			c.Id,
			r.FileName,
			r.Line,
			r.Reason,
			r.FoundBy,
		)
		if err != nil {
			log.Errorf("Error saving %v: %v", r, err)
		}
	}
	if _, err = stmt.Exec(); err != nil {
		return
	}
	if err = stmt.Close(); err != nil {
		return
	}
	if err = txn.Commit(); err != nil {
		return
	}
	log.Debugf("%v: Inserted %d tool results\n", c, len(c.ToolResults))
	return
}

func PersistFunctions(c *Commit) (err error) {
	txn, err := DB.Db.Begin()
	if err != nil {
		return
	}
	// clear old functions
	_, err = txn.Exec("DELETE FROM unstable.functions WHERE commit_id = $1", c.Id)
	if err != nil {
		return fmt.Errorf("%v: deleting old functions falied: %v", c, err)
	}

	// prepare insert
	stmt, err := txn.Prepare(pq.CopyInSchema("unstable", "functions", "commit_id", "name", "file_name", "start_line", "end_line", "state"))
	if err != nil {
		return
	}

	for _, f := range c.Functions {
		_, err = stmt.Exec(
			c.Id,
			f.Name,
			f.FileName,
			f.StartLine,
			f.EndLine,
			f.State,
		)
		if err != nil {
			log.Errorf("Error saving %v: %v", f, err)
		}
	}
	if _, err = stmt.Exec(); err != nil {
		return
	}
	if err = stmt.Close(); err != nil {
		return
	}
	if err = txn.Commit(); err != nil {
		return
	}
	return
}

func tableName(obj DbObj) (string, error) {
	f, ok := reflect.TypeOf(obj).Elem().FieldByName("Id")
	if !ok {
		return "", fmt.Errorf("Field Id not found")
	}
	return f.Tag.Get("table"), nil
}
