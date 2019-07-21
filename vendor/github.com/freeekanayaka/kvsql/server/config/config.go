package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/pkg/errors"
)

// Config holds the server configuraton loaded from disk.
type Config struct {
	KeyPair tls.Certificate
	Pool    *x509.CertPool
	Init    *Init   // Initialization parameters, for new servers.
	Address string  // Server address
	Update  *Update // Configuration updates
}

// Load current the configuration from disk.
func Load(dir string) (*Config, error) {
	// Migrate the legacy node store.
	if err := migrateNodeStore(dir); err != nil {
		return nil, err
	}

	// Load the TLS certificates.
	crt := filepath.Join(dir, "cluster.crt")
	key := filepath.Join(dir, "cluster.key")

	keypair, err := tls.LoadX509KeyPair(crt, key)
	if err != nil {
		return nil, errors.Wrap(err, "load keypair")
	}

	data, err := ioutil.ReadFile(crt)
	if err != nil {
		return nil, errors.Wrap(err, "read certificate")
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("bad certificate")
	}

	// Check if we're initializing a new node (i.e. there's an init.yaml).
	init, err := loadInit(dir)
	if err != nil {
		return nil, err
	}

	var update *Update
	if init == nil {
		update, err = loadUpdate(dir)
		if err != nil {
			return nil, err
		}
	}

	config := &Config{
		KeyPair: keypair,
		Pool:    pool,
		Init:    init,
		Update:  update,
	}

	return config, nil
}
