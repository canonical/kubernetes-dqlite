package server

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/canonical/go-dqlite"
	"github.com/canonical/go-dqlite/app"
	"github.com/canonical/go-dqlite/client"
	"github.com/canonical/kvsql-dqlite/server/config"
	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"github.com/rancher/kine/pkg/endpoint"
)

// Server sets up a single dqlite node and serves the cluster management API.
type Server struct {
	dir        string // Data directory
	address    string // Network address
	app        *app.App
	cancelKine context.CancelFunc
}

func New(dir string) (*Server, error) {
	// Check if we're initializing a new node (i.e. there's an init.yaml).
	cfg, err := config.Load(dir)
	if err != nil {
		return nil, err
	}

	if cfg.Update != nil {
		info := client.NodeInfo{}
		path := filepath.Join(dir, "info.yaml")
		data, err := ioutil.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if err := yaml.Unmarshal(data, &info); err != nil {
			return nil, err
		}
		info.Address = cfg.Update.Address
		data, err = yaml.Marshal(info)
		if err != nil {
			return nil, err
		}
		if err := ioutil.WriteFile(path, data, 0600); err != nil {
			return nil, err
		}
		nodes := []dqlite.NodeInfo{info}
		if err := dqlite.ReconfigureMembership(dir, nodes); err != nil {
			return nil, err
		}
		store, err := client.NewYamlNodeStore(filepath.Join(dir, "cluster.yaml"))
		if err != nil {
			return nil, err
		}
		if err := store.Set(context.Background(), nodes); err != nil {
			return nil, err
		}
		if err := os.Remove(filepath.Join(dir, "update.yaml")); err != nil {
			return nil, errors.Wrap(err, "remove update.yaml")
		}
	}

	options := []app.Option{
		app.WithTLS(app.SimpleTLSConfig(cfg.KeyPair, cfg.Pool)),
		app.WithFailureDomain(cfg.FailureDomain),
	}

	// Possibly initialize our ID, address and initial node store content.
	if cfg.Init != nil {
		options = append(options, app.WithAddress(cfg.Init.Address), app.WithCluster(cfg.Init.Cluster))
	}

	app, err := app.New(dir, options...)
	if err != nil {
		return nil, err
	}
	if cfg.Init != nil {
		if err := os.Remove(filepath.Join(dir, "init.yaml")); err != nil {
			return nil, err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if err := app.Ready(ctx); err != nil {
		return nil, err
	}

	socket := filepath.Join(dir, "kine.sock")
	peers := filepath.Join(dir, "cluster.yaml")
	config := endpoint.Config{
		Listener: fmt.Sprintf("unix://%s", socket),
		Endpoint: fmt.Sprintf("dqlite://k8s?peer-file=%s&driver-name=%s", peers, app.Driver()),
	}
	kineCtx, cancelKine := context.WithCancel(context.Background())
	if _, err := endpoint.Listen(kineCtx, config); err != nil {
		return nil, errors.Wrap(err, "kine")
	}

	s := &Server{
		dir:        dir,
		address:    cfg.Address,
		app:        app,
		cancelKine: cancelKine,
	}

	return s, nil
}

func (s *Server) Close(ctx context.Context) error {
	if s.cancelKine != nil {
		s.cancelKine()
	}
	s.app.Handover(ctx)
	if err := s.app.Close(); err != nil {
		return errors.Wrap(err, "stop dqlite app")
	}
	return nil
}
