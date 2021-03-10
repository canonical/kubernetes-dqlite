package client_test

import (
	"context"
	"testing"

	"github.com/canonical/go-dqlite/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exercise setting and getting servers in a DatabaseNodeStore created with
// DefaultNodeStore.
func TestDefaultNodeStore(t *testing.T) {
	// Create a new default store.
	store, err := client.DefaultNodeStore(":memory:")
	require.NoError(t, err)

	// Set and get some targets.
	err = store.Set(context.Background(), []client.NodeInfo{
		{Address: "1.2.3.4:666"}, {Address: "5.6.7.8:666"}},
	)
	require.NoError(t, err)

	servers, err := store.Get(context.Background())
	assert.Equal(t, []client.NodeInfo{
		{ID: uint64(1), Address: "1.2.3.4:666"},
		{ID: uint64(1), Address: "5.6.7.8:666"}},
		servers)

	// Set and get some new targets.
	err = store.Set(context.Background(), []client.NodeInfo{
		{Address: "1.2.3.4:666"}, {Address: "9.9.9.9:666"},
	})
	require.NoError(t, err)

	servers, err = store.Get(context.Background())
	assert.Equal(t, []client.NodeInfo{
		{ID: uint64(1), Address: "1.2.3.4:666"},
		{ID: uint64(1), Address: "9.9.9.9:666"}},
		servers)

	// Setting duplicate targets returns an error and the change is not
	// persisted.
	err = store.Set(context.Background(), []client.NodeInfo{
		{Address: "1.2.3.4:666"}, {Address: "1.2.3.4:666"},
	})
	assert.EqualError(t, err, "failed to insert server 1.2.3.4:666: UNIQUE constraint failed: servers.address")

	servers, err = store.Get(context.Background())
	assert.Equal(t, []client.NodeInfo{
		{ID: uint64(1), Address: "1.2.3.4:666"},
		{ID: uint64(1), Address: "9.9.9.9:666"}},
		servers)
}
