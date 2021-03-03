package factory_test

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/canonical/kvsql-dqlite/server"
	"github.com/canonical/kvsql-dqlite/server/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/api/apitesting"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apiserver/pkg/apis/example"
	examplev1 "k8s.io/apiserver/pkg/apis/example/v1"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/storagebackend"
	"k8s.io/apiserver/pkg/storage/storagebackend/factory"
)

func TestCreate_First(t *testing.T) {
	store, cleanup := newStore(t)
	defer cleanup()

	ctx := context.Background()

	out := &example.Pod{}
	obj := &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo", SelfLink: "testlink"}}
	err := store.Create(ctx, "/foo", obj, out, uint64(0))
	require.NoError(t, err)
	err = store.Create(ctx, "/bar", obj, out, uint64(0))
	require.NoError(t, err)
}

func TestCreate_Existing(t *testing.T) {
	store, cleanup := newStore(t)
	defer cleanup()

	ctx := context.Background()

	out := &example.Pod{}
	obj := &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo", SelfLink: "testlink"}}
	require.NoError(t, store.Create(ctx, "foo", obj, out, uint64(0)))

	err := store.Create(ctx, "/foo", obj, out, uint64(0))
	if err, ok := err.(*storage.StorageError); ok {
		assert.Equal(t, err.Code, storage.ErrCodeKeyExists)
		assert.Equal(t, err.Key, "/foo")
	} else {
		t.Fatalf("Unexpected error: %v", err)
	}
}

func TestCreate_Concurrent(t *testing.T) {
	store, cleanup := newStore(t)
	defer cleanup()

	ctx := context.Background()

	errors := make(chan error, 0)

	go func() {
		out := &example.Pod{}
		obj := &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo", SelfLink: "testlink"}}
		errors <- store.Create(ctx, "foo", obj, out, uint64(0))
	}()

	go func() {
		out := &example.Pod{}
		obj := &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "bar", SelfLink: "testlink"}}
		errors <- store.Create(ctx, "bar", obj, out, uint64(0))
	}()

	require.NoError(t, <-errors)
	require.NoError(t, <-errors)
}

func TestCreateAgainAfterDeletion(t *testing.T) {
	store, cleanup := newStore(t)
	defer cleanup()

	ctx := context.Background()

	out := &example.Pod{}
	obj := &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo", SelfLink: "testlink"}}
	require.NoError(t, store.Create(ctx, "foo", obj, out, uint64(0)))

	err := store.Delete(ctx, "/foo", obj, nil, func(context.Context, runtime.Object) error { return nil })
	require.NoError(t, err)

	obj = &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo", SelfLink: "testlink"}}
	require.NoError(t, store.Create(ctx, "foo", obj, out, uint64(0)))
}

var scheme = runtime.NewScheme()
var codecs = serializer.NewCodecFactory(scheme)

func init() {
	metav1.AddToGroupVersion(scheme, metav1.SchemeGroupVersion)
	utilruntime.Must(example.AddToScheme(scheme))
	utilruntime.Must(examplev1.AddToScheme(scheme))
}

func newStore(t testing.TB) (storage.Interface, func()) {
	init := &config.Init{Address: "localhost:9991"}
	dir, dirCleanup := newDirWithInit(t, init)

	server, err := server.New(dir)
	require.NoError(t, err)

	codec := apitesting.TestCodec(codecs, examplev1.SchemeGroupVersion)

	config := storagebackend.Config{
		Codec: codec,
		Dir:   dir,
		Type:  storagebackend.StorageTypeDqlite,
	}

	store, destroy, err := factory.Create(config)
	require.NoError(t, err)

	cleanup := func() {
		destroy()
		server.Close(context.Background())
		dirCleanup()
	}

	return store, cleanup

}

// Return a new temporary directory populated with the test cluster certificate
// and an init.yaml file with the given content.
func newDirWithInit(t testing.TB, init *config.Init) (string, func()) {
	dir, cleanup := newDirWithCert(t)

	path := filepath.Join(dir, "init.yaml")
	bytes, err := yaml.Marshal(init)
	require.NoError(t, err)
	require.NoError(t, ioutil.WriteFile(path, bytes, 0644))

	return dir, cleanup
}

// Return a new temporary directory populated with the test cluster certificate.
func newDirWithCert(t testing.TB) (string, func()) {
	t.Helper()

	dir, cleanup := newDir(t)

	// Create symlinks to the test certificates.
	for _, filename := range []string{"cluster.crt", "cluster.key"} {
		link := filepath.Join(dir, filename)
		target, err := filepath.Abs(filepath.Join("server/testdata", filename))
		require.NoError(t, err)
		require.NoError(t, os.Symlink(target, link))
	}

	return dir, cleanup
}

// Return a new temporary directory.
func newDir(t testing.TB) (string, func()) {
	t.Helper()

	dir, err := ioutil.TempDir("", "kvsql-server-test-")
	require.NoError(t, err)

	cleanup := func() {
		require.NoError(t, os.RemoveAll(dir))
	}

	return dir, cleanup
}
