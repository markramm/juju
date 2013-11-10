// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"crypto/tls"
	"crypto/x509"
	stderrors "errors"
	"fmt"
	"net"
	"time"

	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"

	"github.com/jameinel/juju/cert"
	"github.com/jameinel/juju/constraints"
	"github.com/jameinel/juju/environs/config"
	"github.com/jameinel/juju/errors"
	"github.com/jameinel/juju/state/presence"
	"github.com/jameinel/juju/state/watcher"
	"github.com/jameinel/juju/utils"
)

// Info encapsulates information about cluster of
// servers holding juju state and can be used to make a
// connection to that cluster.
type Info struct {
	// Addrs gives the addresses of the MongoDB servers for the state.
	// Each address should be in the form address:port.
	Addrs []string

	// CACert holds the CA certificate that will be used
	// to validate the state server's certificate, in PEM format.
	CACert []byte

	// Tag holds the name of the entity that is connecting.
	// It should be empty when connecting as an administrator.
	Tag string

	// Password holds the password for the connecting entity.
	Password string
}

// DialOpts holds configuration parameters that control the
// Dialing behavior when connecting to a state server.
type DialOpts struct {
	// Timeout is the amount of time to wait contacting
	// a state server.
	Timeout time.Duration
}

// DefaultDialOpts returns a DialOpts representing the default
// parameters for contacting a state server.
func DefaultDialOpts() DialOpts {
	return DialOpts{
		Timeout: 10 * time.Minute,
	}
}

// Open connects to the server described by the given
// info, waits for it to be initialized, and returns a new State
// representing the environment connected to.
// It returns unauthorizedError if access is unauthorized.
func Open(info *Info, opts DialOpts) (*State, error) {
	logger.Infof("opening state; mongo addresses: %q; entity %q", info.Addrs, info.Tag)
	if len(info.Addrs) == 0 {
		return nil, stderrors.New("no mongo addresses")
	}
	if len(info.CACert) == 0 {
		return nil, stderrors.New("missing CA certificate")
	}
	xcert, err := cert.ParseCert(info.CACert)
	if err != nil {
		return nil, fmt.Errorf("cannot parse CA certificate: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(xcert)
	tlsConfig := &tls.Config{
		RootCAs:    pool,
		ServerName: "anything",
	}
	dial := func(addr net.Addr) (net.Conn, error) {
		c, err := net.Dial("tcp", addr.String())
		if err != nil {
			logger.Debugf("connection failed, will retry: %v", err)
			return nil, err
		}
		cc := tls.Client(c, tlsConfig)
		if err := cc.Handshake(); err != nil {
			logger.Errorf("TLS handshake failed: %v", err)
			return nil, err
		}
		return cc, nil
	}
	session, err := mgo.DialWithInfo(&mgo.DialInfo{
		Addrs:   info.Addrs,
		Timeout: opts.Timeout,
		Dial:    dial,
	})
	if err != nil {
		return nil, err
	}
	logger.Infof("connection established")
	st, err := newState(session, info)
	if err != nil {
		session.Close()
		return nil, err
	}
	return st, nil
}

// Initialize sets up an initial empty state and returns it.
// This needs to be performed only once for a given environment.
// It returns unauthorizedError if access is unauthorized.
func Initialize(info *Info, cfg *config.Config, opts DialOpts) (rst *State, err error) {
	st, err := Open(info, opts)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			st.Close()
		}
	}()
	// A valid environment is used as a signal that the
	// state has already been initalized. If this is the case
	// do nothing.
	if _, err := st.Environment(); err == nil {
		return st, nil
	} else if !errors.IsNotFoundError(err) {
		return nil, err
	}
	logger.Infof("initializing environment")
	if err := checkEnvironConfig(cfg); err != nil {
		return nil, err
	}
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, fmt.Errorf("environment UUID cannot be created: %v", err)
	}
	ops := []txn.Op{
		createConstraintsOp(st, environGlobalKey, constraints.Value{}),
		createSettingsOp(st, environGlobalKey, cfg.AllAttrs()),
		createEnvironmentOp(st, cfg.Name(), uuid.String()),
	}
	if err := st.runTransaction(ops); err == txn.ErrAborted {
		// The config was created in the meantime.
		return st, nil
	} else if err != nil {
		return nil, err
	}
	return st, nil
}

var indexes = []struct {
	collection string
	key        []string
}{
	// After the first public release, do not remove entries from here
	// without adding them to a list of indexes to drop, to ensure
	// old databases are modified to have the correct indexes.
	{"relations", []string{"endpoints.relationname"}},
	{"relations", []string{"endpoints.servicename"}},
	{"units", []string{"service"}},
	{"units", []string{"principal"}},
	{"units", []string{"machineid"}},
	{"users", []string{"name"}},
}

// The capped collection used for transaction logs defaults to 10MB.
// It's tweaked in export_test.go to 1MB to avoid the overhead of
// creating and deleting the large file repeatedly in tests.
var (
	logSize      = 10000000
	logSizeTests = 1000000
)

func maybeUnauthorized(err error, msg string) error {
	if err == nil {
		return nil
	}
	if isUnauthorized(err) {
		return errors.Unauthorizedf("%s: unauthorized mongo access: %v", msg, err)
	}
	return fmt.Errorf("%s: %v", msg, err)
}

func isUnauthorized(err error) bool {
	if err == nil {
		return false
	}
	// Some unauthorized access errors have no error code,
	// just a simple error string.
	if err.Error() == "auth fails" {
		return true
	}
	if err, ok := err.(*mgo.QueryError); ok {
		return err.Code == 10057 ||
			err.Message == "need to login" ||
			err.Message == "unauthorized"
	}
	return false
}

func newState(session *mgo.Session, info *Info) (*State, error) {
	db := session.DB("juju")
	pdb := session.DB("presence")
	if info.Tag != "" {
		if err := db.Login(info.Tag, info.Password); err != nil {
			return nil, maybeUnauthorized(err, fmt.Sprintf("cannot log in to juju database as %q", info.Tag))
		}
		if err := pdb.Login(info.Tag, info.Password); err != nil {
			return nil, maybeUnauthorized(err, fmt.Sprintf("cannot log in to presence database as %q", info.Tag))
		}
	} else if info.Password != "" {
		admin := session.DB("admin")
		if err := admin.Login("admin", info.Password); err != nil {
			return nil, maybeUnauthorized(err, "cannot log in to admin database")
		}
	}
	st := &State{
		info:           info,
		db:             db,
		environments:   db.C("environments"),
		charms:         db.C("charms"),
		machines:       db.C("machines"),
		containerRefs:  db.C("containerRefs"),
		instanceData:   db.C("instanceData"),
		relations:      db.C("relations"),
		relationScopes: db.C("relationscopes"),
		services:       db.C("services"),
		minUnits:       db.C("minunits"),
		settings:       db.C("settings"),
		settingsrefs:   db.C("settingsrefs"),
		constraints:    db.C("constraints"),
		units:          db.C("units"),
		users:          db.C("users"),
		presence:       pdb.C("presence"),
		cleanups:       db.C("cleanups"),
		annotations:    db.C("annotations"),
		statuses:       db.C("statuses"),
	}
	log := db.C("txns.log")
	logInfo := mgo.CollectionInfo{Capped: true, MaxBytes: logSize}
	// The lack of error code for this error was reported upstream:
	//     https://jira.klmongodb.org/browse/SERVER-6992
	err := log.Create(&logInfo)
	if err != nil && err.Error() != "collection already exists" {
		return nil, maybeUnauthorized(err, "cannot create log collection")
	}
	st.runner = txn.NewRunner(db.C("txns"))
	st.runner.ChangeLog(db.C("txns.log"))
	st.watcher = watcher.New(db.C("txns.log"))
	st.pwatcher = presence.NewWatcher(pdb.C("presence"))
	for _, item := range indexes {
		index := mgo.Index{Key: item.key}
		if err := db.C(item.collection).EnsureIndex(index); err != nil {
			return nil, fmt.Errorf("cannot create database index: %v", err)
		}
	}
	st.transactionHooks = make(chan ([]transactionHook), 1)
	st.transactionHooks <- nil
	return st, nil
}

// CACert returns the certificate used to validate the state connection.
func (st *State) CACert() (cert []byte) {
	return append(cert, st.info.CACert...)
}

func (st *State) Close() error {
	err1 := st.watcher.Stop()
	err2 := st.pwatcher.Stop()
	st.mu.Lock()
	var err3 error
	if st.allManager != nil {
		err3 = st.allManager.Stop()
	}
	st.mu.Unlock()
	st.db.Session.Close()
	for _, err := range []error{err1, err2, err3} {
		if err != nil {
			return err
		}
	}
	return nil
}
