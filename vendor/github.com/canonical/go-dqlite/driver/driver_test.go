// Copyright 2017 Canonical Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package driver_test

import (
	"context"
	"database/sql/driver"
	"io"
	"io/ioutil"
	"os"
	"testing"

	dqlite "github.com/canonical/go-dqlite"
	"github.com/canonical/go-dqlite/client"
	dqlitedriver "github.com/canonical/go-dqlite/driver"
	"github.com/canonical/go-dqlite/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDriver_Open(t *testing.T) {
	driver, cleanup := newDriver(t)
	defer cleanup()

	conn, err := driver.Open("test.db")
	require.NoError(t, err)

	assert.NoError(t, conn.Close())
}

func TestDriver_Prepare(t *testing.T) {
	driver, cleanup := newDriver(t)
	defer cleanup()

	conn, err := driver.Open("test.db")
	require.NoError(t, err)

	stmt, err := conn.Prepare("CREATE TABLE test (n INT)")
	require.NoError(t, err)

	assert.Equal(t, 0, stmt.NumInput())

	assert.NoError(t, conn.Close())
}

func TestConn_Exec(t *testing.T) {
	drv, cleanup := newDriver(t)
	defer cleanup()

	conn, err := drv.Open("test.db")
	require.NoError(t, err)

	_, err = conn.Begin()
	require.NoError(t, err)

	execer := conn.(driver.Execer)

	_, err = execer.Exec("CREATE TABLE test (n INT)", nil)
	require.NoError(t, err)

	result, err := execer.Exec("INSERT INTO test(n) VALUES(1)", nil)
	require.NoError(t, err)

	lastInsertID, err := result.LastInsertId()
	require.NoError(t, err)

	assert.Equal(t, lastInsertID, int64(1))

	rowsAffected, err := result.RowsAffected()
	require.NoError(t, err)

	assert.Equal(t, rowsAffected, int64(1))

	assert.NoError(t, conn.Close())
}

func TestConn_Query(t *testing.T) {
	drv, cleanup := newDriver(t)
	defer cleanup()

	conn, err := drv.Open("test.db")
	require.NoError(t, err)

	_, err = conn.Begin()
	require.NoError(t, err)

	execer := conn.(driver.Execer)

	_, err = execer.Exec("CREATE TABLE test (n INT)", nil)
	require.NoError(t, err)

	_, err = execer.Exec("INSERT INTO test(n) VALUES(1)", nil)
	require.NoError(t, err)

	queryer := conn.(driver.Queryer)

	_, err = queryer.Query("SELECT n FROM test", nil)
	require.NoError(t, err)

	assert.NoError(t, conn.Close())
}

func TestConn_QueryRow(t *testing.T) {
	drv, cleanup := newDriver(t)
	defer cleanup()

	conn, err := drv.Open("test.db")
	require.NoError(t, err)

	_, err = conn.Begin()
	require.NoError(t, err)

	execer := conn.(driver.Execer)

	_, err = execer.Exec("CREATE TABLE test (n INT)", nil)
	require.NoError(t, err)

	_, err = execer.Exec("INSERT INTO test(n) VALUES(1)", nil)
	require.NoError(t, err)

	_, err = execer.Exec("INSERT INTO test(n) VALUES(1)", nil)
	require.NoError(t, err)

	queryer := conn.(driver.Queryer)

	rows, err := queryer.Query("SELECT n FROM test", nil)
	require.NoError(t, err)

	values := make([]driver.Value, 1)
	require.NoError(t, rows.Next(values))

	require.NoError(t, rows.Close())

	assert.NoError(t, conn.Close())
}

func TestConn_QueryBlob(t *testing.T) {
	drv, cleanup := newDriver(t)
	defer cleanup()

	conn, err := drv.Open("test.db")
	require.NoError(t, err)

	_, err = conn.Begin()
	require.NoError(t, err)

	execer := conn.(driver.Execer)

	_, err = execer.Exec("CREATE TABLE test (data BLOB)", nil)
	require.NoError(t, err)

	values := []driver.Value{
		[]byte{'a', 'b', 'c'},
	}
	_, err = execer.Exec("INSERT INTO test(data) VALUES(?)", values)
	require.NoError(t, err)

	queryer := conn.(driver.Queryer)

	rows, err := queryer.Query("SELECT data FROM test", nil)
	require.NoError(t, err)

	assert.Equal(t, rows.Columns(), []string{"data"})

	values = make([]driver.Value, 1)
	require.NoError(t, rows.Next(values))

	assert.Equal(t, []byte{'a', 'b', 'c'}, values[0])

	assert.NoError(t, conn.Close())
}

func TestStmt_Exec(t *testing.T) {
	drv, cleanup := newDriver(t)
	defer cleanup()

	conn, err := drv.Open("test.db")
	require.NoError(t, err)

	stmt, err := conn.Prepare("CREATE TABLE test (n INT)")
	require.NoError(t, err)

	_, err = conn.Begin()
	require.NoError(t, err)

	_, err = stmt.Exec(nil)
	require.NoError(t, err)

	require.NoError(t, stmt.Close())

	values := []driver.Value{
		int64(1),
	}

	stmt, err = conn.Prepare("INSERT INTO test(n) VALUES(?)")
	require.NoError(t, err)

	result, err := stmt.Exec(values)
	require.NoError(t, err)

	lastInsertID, err := result.LastInsertId()
	require.NoError(t, err)

	assert.Equal(t, lastInsertID, int64(1))

	rowsAffected, err := result.RowsAffected()
	require.NoError(t, err)

	assert.Equal(t, rowsAffected, int64(1))

	require.NoError(t, stmt.Close())

	assert.NoError(t, conn.Close())
}

func TestStmt_Query(t *testing.T) {
	drv, cleanup := newDriver(t)
	defer cleanup()

	conn, err := drv.Open("test.db")
	require.NoError(t, err)

	stmt, err := conn.Prepare("CREATE TABLE test (n INT)")
	require.NoError(t, err)

	_, err = conn.Begin()
	require.NoError(t, err)

	_, err = stmt.Exec(nil)
	require.NoError(t, err)

	require.NoError(t, stmt.Close())

	stmt, err = conn.Prepare("INSERT INTO test(n) VALUES(-123)")
	require.NoError(t, err)

	_, err = stmt.Exec(nil)
	require.NoError(t, err)

	require.NoError(t, stmt.Close())

	stmt, err = conn.Prepare("SELECT n FROM test")
	require.NoError(t, err)

	rows, err := stmt.Query(nil)
	require.NoError(t, err)

	assert.Equal(t, rows.Columns(), []string{"n"})

	values := make([]driver.Value, 1)
	require.NoError(t, rows.Next(values))

	assert.Equal(t, int64(-123), values[0])

	require.Equal(t, io.EOF, rows.Next(values))

	require.NoError(t, stmt.Close())

	assert.NoError(t, conn.Close())
}

func TestConn_QueryParams(t *testing.T) {
	drv, cleanup := newDriver(t)
	defer cleanup()

	conn, err := drv.Open("test.db")
	require.NoError(t, err)

	_, err = conn.Begin()
	require.NoError(t, err)

	execer := conn.(driver.Execer)

	_, err = execer.Exec("CREATE TABLE test (n INT, t TEXT)", nil)
	require.NoError(t, err)

	_, err = execer.Exec(`
INSERT INTO test (n,t) VALUES (1,'a');
INSERT INTO test (n,t) VALUES (2,'a');
INSERT INTO test (n,t) VALUES (2,'b');
INSERT INTO test (n,t) VALUES (3,'b');
`,
		nil)
	require.NoError(t, err)

	values := []driver.Value{
		int64(1),
		"a",
	}

	queryer := conn.(driver.Queryer)

	rows, err := queryer.Query("SELECT n, t FROM test WHERE n > ? AND t = ?", values)
	require.NoError(t, err)

	assert.Equal(t, rows.Columns()[0], "n")

	values = make([]driver.Value, 2)
	require.NoError(t, rows.Next(values))

	assert.Equal(t, int64(2), values[0])
	assert.Equal(t, "a", values[1])

	require.Equal(t, io.EOF, rows.Next(values))

	assert.NoError(t, conn.Close())
}

func Test_ColumnTypesEmpty(t *testing.T) {
	t.Skip("this currently fails if the result set is empty, is dqlite skipping the header if empty set?")
	drv, cleanup := newDriver(t)
	defer cleanup()

	conn, err := drv.Open("test.db")
	require.NoError(t, err)

	stmt, err := conn.Prepare("CREATE TABLE test (n INT)")
	require.NoError(t, err)

	_, err = conn.Begin()
	require.NoError(t, err)

	_, err = stmt.Exec(nil)
	require.NoError(t, err)

	require.NoError(t, stmt.Close())

	stmt, err = conn.Prepare("SELECT n FROM test")
	require.NoError(t, err)

	rows, err := stmt.Query(nil)
	require.NoError(t, err)

	require.NoError(t, err)
	rowTypes, ok := rows.(driver.RowsColumnTypeDatabaseTypeName)
	require.True(t, ok)

	typeName := rowTypes.ColumnTypeDatabaseTypeName(0)
	assert.Equal(t, "INTEGER", typeName)

	require.NoError(t, stmt.Close())

	assert.NoError(t, conn.Close())
}

func Test_ColumnTypesExists(t *testing.T) {
	drv, cleanup := newDriver(t)
	defer cleanup()

	conn, err := drv.Open("test.db")
	require.NoError(t, err)

	stmt, err := conn.Prepare("CREATE TABLE test (n INT)")
	require.NoError(t, err)

	_, err = conn.Begin()
	require.NoError(t, err)

	_, err = stmt.Exec(nil)
	require.NoError(t, err)

	require.NoError(t, stmt.Close())

	stmt, err = conn.Prepare("INSERT INTO test(n) VALUES(-123)")
	require.NoError(t, err)

	_, err = stmt.Exec(nil)
	require.NoError(t, err)

	stmt, err = conn.Prepare("SELECT n FROM test")
	require.NoError(t, err)

	rows, err := stmt.Query(nil)
	require.NoError(t, err)

	require.NoError(t, err)
	rowTypes, ok := rows.(driver.RowsColumnTypeDatabaseTypeName)
	require.True(t, ok)

	typeName := rowTypes.ColumnTypeDatabaseTypeName(0)
	assert.Equal(t, "INTEGER", typeName)

	require.NoError(t, stmt.Close())
	assert.NoError(t, conn.Close())
}

// ensure column types data is available
// even after the last row of the query
func Test_ColumnTypesEnd(t *testing.T) {
	drv, cleanup := newDriver(t)
	defer cleanup()

	conn, err := drv.Open("test.db")
	require.NoError(t, err)

	stmt, err := conn.Prepare("CREATE TABLE test (n INT)")
	require.NoError(t, err)

	_, err = conn.Begin()
	require.NoError(t, err)

	_, err = stmt.Exec(nil)
	require.NoError(t, err)

	require.NoError(t, stmt.Close())

	stmt, err = conn.Prepare("INSERT INTO test(n) VALUES(-123)")
	require.NoError(t, err)

	_, err = stmt.Exec(nil)
	require.NoError(t, err)

	stmt, err = conn.Prepare("SELECT n FROM test")
	require.NoError(t, err)

	rows, err := stmt.Query(nil)
	require.NoError(t, err)

	require.NoError(t, err)
	rowTypes, ok := rows.(driver.RowsColumnTypeDatabaseTypeName)
	require.True(t, ok)

	typeName := rowTypes.ColumnTypeDatabaseTypeName(0)
	assert.Equal(t, "INTEGER", typeName)

	values := make([]driver.Value, 1)
	require.NoError(t, rows.Next(values))

	assert.Equal(t, int64(-123), values[0])

	require.Equal(t, io.EOF, rows.Next(values))

	// despite EOF we should have types cached
	typeName = rowTypes.ColumnTypeDatabaseTypeName(0)
	assert.Equal(t, "INTEGER", typeName)

	require.NoError(t, stmt.Close())
	assert.NoError(t, conn.Close())
}

func newDriver(t *testing.T) (*dqlitedriver.Driver, func()) {
	t.Helper()

	_, cleanup := newNode(t)

	store := newStore(t, "@1")

	log := logging.Test(t)

	driver, err := dqlitedriver.New(store, dqlitedriver.WithLogFunc(log))
	require.NoError(t, err)

	return driver, cleanup
}

// Create a new in-memory server store populated with the given addresses.
func newStore(t *testing.T, address string) client.NodeStore {
	t.Helper()

	store, err := client.DefaultNodeStore(":memory:")
	require.NoError(t, err)

	server := client.NodeInfo{Address: address}
	require.NoError(t, store.Set(context.Background(), []client.NodeInfo{server}))

	return store
}

func newNode(t *testing.T) (*dqlite.Node, func()) {
	t.Helper()
	dir, dirCleanup := newDir(t)

	server, err := dqlite.New(uint64(1), "@1", dir, dqlite.WithBindAddress("@1"))
	require.NoError(t, err)

	err = server.Start()
	require.NoError(t, err)

	cleanup := func() {
		require.NoError(t, server.Close())
		dirCleanup()
	}

	return server, cleanup
}

// Return a new temporary directory.
func newDir(t *testing.T) (string, func()) {
	t.Helper()

	dir, err := ioutil.TempDir("", "dqlite-replication-test-")
	assert.NoError(t, err)

	cleanup := func() {
		_, err := os.Stat(dir)
		if err != nil {
			assert.True(t, os.IsNotExist(err))
		} else {
			assert.NoError(t, os.RemoveAll(dir))
		}
	}

	return dir, cleanup
}

/*
import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/canonical/go-dqlite"
	"github.com/CanonicalLtd/go-sqlite3"
	"github.com/CanonicalLtd/raft-test"
	"github.com/hashicorp/raft"
	"github.com/mpvl/subtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Using invalid paths in Config.Dir results in an error.
func TestNewDriver_DirErrors(t *testing.T) {
	cases := []struct {
		title string
		dir   string // Dir to pass to the new driver.
		error string // Expected message
	}{
		{
			`no path given at all`,
			"",
			"no data dir provided in config",
		},
		{
			`non-existing path that can't be created`,
			"/cant/create/anything/here/",
			"failed to create data dir",
		},
		{
			`path that can't be accessed`,
			"/proc/1/root/",
			"failed to access data dir",
		},
		{
			`path that is not a directory`,
			"/etc/fstab",
			"data dir '/etc/fstab' is not a directory",
		},
	}
	for _, c := range cases {
		subtest.Run(t, c.title, func(t *testing.T) {
			registry := dqlite.NewRegistry(c.dir)
			driver, err := dqlite.NewDriver(registry, nil, dqlite.DriverConfig{})
			assert.Nil(t, driver)
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.error)
		})
	}
}

func TestNewDriver_CreateDir(t *testing.T) {
	dir, cleanup := newDir(t)
	defer cleanup()

	dir = filepath.Join(dir, "does", "not", "exist")
	registry := dqlite.NewRegistry(dir)
	_, err := dqlite.NewDriver(registry, &raft.Raft{}, dqlite.DriverConfig{})
	assert.NoError(t, err)
}

func DISABLE_TestDriver_SQLiteLogging(t *testing.T) {
	output := bytes.NewBuffer(nil)
	logger := log.New(output, "", 0)
	config := dqlite.DriverConfig{Logger: logger}

	driver, cleanup := newDriverWithConfig(t, config)
	defer cleanup()

	conn, err := driver.Open("test.db")
	require.NoError(t, err)

	_, err = conn.Prepare("CREATE FOO")
	require.Error(t, err)
	assert.Contains(t, output.String(), `[ERR] near "FOO": syntax error (1)`)
}

func TestDriver_OpenClose(t *testing.T) {
	driver, cleanup := newDriver(t)
	defer cleanup()

	conn, err := driver.Open("test.db")
	require.NoError(t, err)
	assert.NoError(t, conn.Close())
}

func TestDriver_OpenInvalidURI(t *testing.T) {
	driver, cleanup := newDriver(t)
	defer cleanup()

	conn, err := driver.Open("/foo/test.db")
	assert.Nil(t, conn)
	assert.EqualError(t, err, "invalid URI /foo/test.db: directory segments are invalid")
}

func TestDriver_OpenError(t *testing.T) {
	dir, cleanup := newDir(t)
	defer cleanup()

	registry := dqlite.NewRegistry(dir)
	fsm := dqlite.NewFSM(registry)
	raft, cleanup := rafttest.Node(t, fsm)
	defer cleanup()
	config := dqlite.DriverConfig{}

	driver, err := dqlite.NewDriver(registry, raft, config)
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(dir))

	conn, err := driver.Open("test.db")
	assert.Nil(t, conn)

	expected := fmt.Sprintf("open error for %s: unable to open database file", filepath.Join(dir, "test.db"))
	assert.EqualError(t, err, expected)
}

// If the driver is not the current leader, all APIs return an error.
func TestDriver_NotLeader_Errors(t *testing.T) {
	cases := []struct {
		title string
		f     func(*testing.T, *dqlite.Conn) error
	}{
		{
			`open`,
			func(t *testing.T, conn *dqlite.Conn) error {
				_, err := conn.Prepare("CREATE TABLE foo (n INT)")
				return err
			},
		},
		{
			`exec`,
			func(t *testing.T, conn *dqlite.Conn) error {
				_, err := conn.Exec("CREATE TABLE foo (n INT)", nil)
				return err
			},
		},
		{
			`begin`,
			func(t *testing.T, conn *dqlite.Conn) error {
				_, err := conn.Begin()
				return err
			},
		},
	}

	for _, c := range cases {
		t.Run(c.title, func(t *testing.T) {
			dir, cleanup := newDir(t)
			defer cleanup()

			registry1 := dqlite.NewRegistry(dir)
			registry2 := dqlite.NewRegistry(dir)
			fsm1 := dqlite.NewFSM(registry1)
			fsm2 := dqlite.NewFSM(registry2)
			rafts, control := rafttest.Cluster(t, []raft.FSM{fsm1, fsm2}, rafttest.Latency(1000.0))
			defer control.Close()

			config := dqlite.DriverConfig{}

			driver, err := dqlite.NewDriver(registry1, rafts["0"], config)
			require.NoError(t, err)

			conn, err := driver.Open("test.db")
			require.NoError(t, err)

			err = c.f(t, conn.(*dqlite.Conn))
			require.Error(t, err)
			erri, ok := err.(sqlite3.Error)
			require.True(t, ok)
			assert.Equal(t, sqlite3.ErrIoErrNotLeader, erri.ExtendedCode)
		})
	}
}

// Return the address of the current raft leader.
func TestDriver_Leader(t *testing.T) {
	driver, cleanup := newDriver(t)
	defer cleanup()

	assert.Equal(t, "0", driver.Leader())
}

// Return the addresses of all current raft servers.
func TestDriver_Nodes(t *testing.T) {
	driver, cleanup := newDriver(t)
	defer cleanup()

	servers, err := driver.Nodes()
	require.NoError(t, err)
	assert.Equal(t, []string{"0"}, servers)
}

func TestStmt_Exec(t *testing.T) {
	driver, cleanup := newDriver(t)
	defer cleanup()

	conn, err := driver.Open("test.db")
	require.NoError(t, err)
	defer conn.Close()

	stmt, err := conn.Prepare("CREATE TABLE foo (n INT)")
	require.NoError(t, err)
	_, err = stmt.Exec(nil)
	assert.NoError(t, err)
}

func TestStmt_Query(t *testing.T) {
	driver, cleanup := newDriver(t)
	defer cleanup()

	conn, err := driver.Open("test.db")
	require.NoError(t, err)
	defer conn.Close()

	stmt, err := conn.Prepare("SELECT name FROM sqlite_master")
	require.NoError(t, err)
	assert.Equal(t, 0, stmt.NumInput())
	rows, err := stmt.Query(nil)
	assert.NoError(t, err)
	defer rows.Close()

}

func TestTx_Commit(t *testing.T) {
	driver, cleanup := newDriver(t)
	defer cleanup()

	conn, err := driver.Open("test.db")
	require.NoError(t, err)
	defer conn.Close()

	tx, err := conn.Begin()
	require.NoError(t, err)

	_, err = conn.(*dqlite.Conn).Exec("CREATE TABLE test (n INT)", nil)
	require.NoError(t, err)

	assert.NoError(t, tx.Commit())

	// The transaction ID has been saved in the committed buffer.
	token := tx.(*dqlite.Tx).Token()
	assert.Equal(t, uint64(5), token)
	assert.NoError(t, driver.Recover(token))
}

func TestTx_Rollback(t *testing.T) {
	driver, cleanup := newDriver(t)
	defer cleanup()

	conn, err := driver.Open("test.db")
	require.NoError(t, err)
	defer conn.Close()

	tx, err := conn.Begin()
	require.NoError(t, err)
	assert.NoError(t, tx.Rollback())
}

// Create a new test dqlite.Driver.
func newDriver(t *testing.T) (*dqlite.Driver, func()) {
	config := dqlite.DriverConfig{Logger: newTestingLogger(t, 0)}
	return newDriverWithConfig(t, config)
}

// Create a new test dqlite.Driver with custom configuration.
func newDriverWithConfig(t *testing.T, config dqlite.DriverConfig) (*dqlite.Driver, func()) {
	dir, dirCleanup := newDir(t)

	registry := dqlite.NewRegistry(dir)
	fsm := dqlite.NewFSM(registry)
	raft, raftCleanup := rafttest.Node(t, fsm)

	driver, err := dqlite.NewDriver(registry, raft, config)
	require.NoError(t, err)

	cleanup := func() {
		raftCleanup()
		dirCleanup()
	}

	return driver, cleanup
}

// Create a new test directory and return it, along with a function that can be
// used to remove it.
func newDir(t *testing.T) (string, func()) {
	dir, err := ioutil.TempDir("", "dqlite-driver-test-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	cleanup := func() {
		_, err := os.Stat(dir)
		if err != nil {
			assert.True(t, os.IsNotExist(err))
		} else {
			assert.NoError(t, os.RemoveAll(dir))
		}
	}
	return dir, cleanup
}
*/
