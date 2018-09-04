// Package hostdb provides a HostDB object that implements the renter.hostDB
// interface. The blockchain is scanned for host announcements and hosts that
// are found get added to the host database. The database continually scans the
// set of hosts it has found and updates who is online.
package hostdb

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pachisi456/Sia/modules"
	"github.com/pachisi456/Sia/modules/renter/hostdb/hostdbprofile"
	"github.com/pachisi456/Sia/modules/renter/hostdb/hosttree"
	"github.com/pachisi456/Sia/persist"
	"github.com/pachisi456/Sia/types"

	"github.com/NebulousLabs/threadgroup"

	"github.com/oschwald/geoip2-golang"
)

var (
	// ErrInitialScanIncomplete is returned whenever an operation is not
	// allowed to be executed before the initial host scan has finished.
	ErrInitialScanIncomplete = errors.New("initial hostdb scan is not yet completed")
	errNilCS                 = errors.New("cannot create hostdb with nil consensus set")
	errNilGateway            = errors.New("cannot create hostdb with nil gateway")
)

// Directory and file for ip information database.
//TODO pachisi456: use more straight forward naming
const geolocationDir = "geolocation"
const geolocationFile = "ip-database.mmdb"

// The HostDB is a database of potential hosts. It assigns a weight to each
// host based on their hosting parameters, and then can select hosts at random
// for uploading files.
type HostDB struct {
	// dependencies
	cs         modules.ConsensusSet
	deps       modules.Dependencies
	gateway    modules.Gateway
	log        *persist.Logger
	mu         sync.RWMutex
	persistDir string
	tg         threadgroup.ThreadGroup

	// database with ip information to determine host location
	ipdb *geoip2.Reader

	// hostdbProfiles is the collection of all hostdb profiles the renter created to
	// customize the host selection.
	hostdbProfiles hostdbprofile.HostDBProfiles

	// hostTrees contains a HostTree for each HostDBProfile. The trees are necessary
	// for selecting weighted hosts at random.
	hostTrees hosttree.HostTrees

	// the scanPool is a set of hosts that need to be scanned. There are a
	// handful of goroutines constantly waiting on the channel for hosts to
	// scan. The scan map is used to prevent duplicates from entering the scan
	// pool.
	initialScanComplete  bool
	initialScanLatencies []time.Duration
	scanList             []modules.HostDBEntry
	scanMap              map[string]struct{}
	scanWait             bool
	scanningThreads      int

	blockHeight types.BlockHeight
	lastChange  modules.ConsensusChangeID
}

// New returns a new HostDB.
func New(g modules.Gateway, cs modules.ConsensusSet, persistDir string) (*HostDB, error) {
	// Check for nil inputs.
	if g == nil {
		return nil, errNilGateway
	}
	if cs == nil {
		return nil, errNilCS
	}
	// Create HostDB using production dependencies.
	return NewCustomHostDB(g, cs, persistDir, modules.ProdDependencies)
}

// NewCustomHostDB creates a HostDB using the provided dependencies. It loads the old
// persistence data, spawns the HostDB's scanning threads, and subscribes it to
// the consensusSet.
func NewCustomHostDB(g modules.Gateway, cs modules.ConsensusSet, persistDir string, deps modules.Dependencies) (*HostDB, error) {
	// Create the HostDB object.
	hdb := &HostDB{
		cs:         cs,
		deps:       deps,
		gateway:    g,
		persistDir: persistDir,

		hostdbProfiles: hostdbprofile.NewHostDBProfiles(),

		scanMap: make(map[string]struct{}),
	}

	// add the HostTrees element and initialize it with the default tree
	hdb.mu.Lock()
	hdb.hostTrees = hosttree.NewHostTrees()
	hdb.mu.Unlock()

	// Create the persist directory if it does not yet exist.
	err := os.MkdirAll(persistDir, 0700)
	if err != nil {
		return nil, err
	}

	// Create the logger.
	logger, err := persist.NewFileLogger(filepath.Join(persistDir, "hostdb.log"))
	if err != nil {
		return nil, err
	}
	hdb.log = logger
	err = hdb.tg.AfterStop(func() error {
		if err := hdb.log.Close(); err != nil {
			// Resort to println as the logger is in an uncertain state.
			fmt.Println("Failed to close the hostdb logger:", err)
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Load the ip information database to determine host location (country).
	db, err := geoip2.Open(filepath.Join(persistDir, geolocationDir, geolocationFile))
	if err != nil {
		// Get the geolocation database.
		resp, err := http.Get("http://geolite.maxmind.com/download/geoip/database/GeoLite2-Country.tar.gz")
		if err != nil {
			hdb.log.Print(err)
		}
		// Untar the database.
		r := io.Reader(resp.Body)
		err = UntarGeoLite2(persistDir, r)
		if err != nil {
			hdb.log.Print(err)
		}
		err = nil
		db, err = geoip2.Open(filepath.Join(persistDir, geolocationDir, geolocationFile))
		if err != nil {
			hdb.log.Print(err)
		}
	}
	hdb.ipdb = db

	// Load the prior persistence structures.
	hdb.mu.Lock()
	err, allHosts := hdb.load()
	hdb.mu.Unlock()
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	// Load the host trees, one tree for each hostdb profile.
	hdb.loadHostTrees(allHosts)

	err = hdb.tg.AfterStop(func() error {
		hdb.mu.Lock()
		err := hdb.saveSync()
		hdb.mu.Unlock()
		if err != nil {
			hdb.log.Println("Unable to save the hostdb:", err)
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Loading is complete, establish the save loop.
	go hdb.threadedSaveLoop()

	// Don't perform the remaining startup in the presence of a quitAfterLoad
	// disruption.
	if hdb.deps.Disrupt("quitAfterLoad") {
		return hdb, nil
	}

	// COMPATv1.1.0
	//
	// If the block height has loaded as zero, the most recent consensus change
	// needs to be set to perform a full rescan. This will also help the hostdb
	// to pick up any hosts that it has incorrectly dropped in the past.
	hdb.mu.Lock()
	if hdb.blockHeight == 0 {
		hdb.lastChange = modules.ConsensusChangeBeginning
	}
	hdb.mu.Unlock()

	err = cs.ConsensusSetSubscribe(hdb, hdb.lastChange, hdb.tg.StopChan())
	if err == modules.ErrInvalidConsensusChangeID {
		// Subscribe again using the new ID. This will cause a triggered scan
		// on all of the hosts, but that should be acceptable.
		hdb.mu.Lock()
		hdb.blockHeight = 0
		hdb.lastChange = modules.ConsensusChangeBeginning
		hdb.mu.Unlock()
		err = cs.ConsensusSetSubscribe(hdb, hdb.lastChange, hdb.tg.StopChan())
	}
	if err != nil {
		return nil, errors.New("hostdb subscription failed: " + err.Error())
	}
	err = hdb.tg.OnStop(func() error {
		cs.Unsubscribe(hdb)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Spawn the scan loop during production, but allow it to be disrupted
	// during testing. Primary reason is so that we can fill the hostdb with
	// fake hosts and not have them marked as offline as the scanloop operates.
	if !hdb.deps.Disrupt("disableScanLoop") {
		go hdb.threadedScan()
	} else {
		hdb.initialScanComplete = true
	}

	return hdb, nil
}

// ActiveHosts returns a list of hosts that are currently online, sorted by
// weight. tree specifies the host tree the hosts should be pulled from.
func (hdb *HostDB) ActiveHosts(tree string) (activeHosts []modules.HostDBEntry) {
	allHosts := hdb.hostTrees.All(tree)
	for _, entry := range allHosts {
		if len(entry.ScanHistory) == 0 {
			continue
		}
		if !entry.ScanHistory[len(entry.ScanHistory)-1].Success {
			continue
		}
		if !entry.AcceptingContracts {
			continue
		}
		activeHosts = append(activeHosts, entry)
	}
	return activeHosts
}

// AddHostDBProfiles adds a new hostdb profile to HostDBProfiles.
func (hdb *HostDB) AddHostDBProfiles(name string, storagetier string) (err error) {
	// add profile
	err = hdb.hostdbProfiles.AddHostDBProfile(name, storagetier)
	if err != nil {
		return err
	}

	// save to persistence data
	hdb.mu.Lock()
	err = hdb.saveSync()
	hdb.mu.Unlock()
	if err != nil {
		hdb.log.Println("Unable to save the hostdb profile:", err)
	}
	return
}

// AllHosts returns all of the hosts of the specified host tree, including the
// inactive ones.
func (hdb *HostDB) AllHosts(tree string) (allHosts []modules.HostDBEntry) {
	return hdb.hostTrees.All(tree)
}

// AverageContractPrice returns the average price of a host.
func (hdb *HostDB) AverageContractPrice(tree string) (totalPrice types.Currency) {
	sampleSize := 32
	//TODO pachisi456: add support for multiple profiles / trees
	hosts := hdb.hostTrees.SelectRandom(tree, sampleSize, nil)
	if len(hosts) == 0 {
		return totalPrice
	}
	for _, host := range hosts {
		totalPrice = totalPrice.Add(host.ContractPrice)
	}
	return totalPrice.Div64(uint64(len(hosts)))
}

// Close closes the hostdb, terminating its scanning threads
func (hdb *HostDB) Close() error {
	return hdb.tg.Stop()
}

// Host returns the HostSettings associated with the specified NetAddress. If
// no matching host is found, Host returns false.
func (hdb *HostDB) Host(spk types.SiaPublicKey) (modules.HostDBEntry, bool) {
	host, exists := hdb.hostTrees.Select(spk)
	if !exists {
		return host, exists
	}
	hdb.mu.RLock()
	updateHostHistoricInteractions(&host, hdb.blockHeight)
	hdb.mu.RUnlock()
	return host, exists
}

// HostDBProfiles returns the map of all set hostdb profiles.
func (hdb *HostDB) HostDBProfiles() (hdbp map[string]*hostdbprofile.HostDBProfile) {
	return hdb.hostdbProfiles.HostDBProfiles()
}

// HostDBProfile returns the hostdb profile with the given name.
func (hdb *HostDB) HostDBProfile(name string) hostdbprofile.HostDBProfile {
	return hdb.hostdbProfiles.GetProfile(name)
}

// ConfigHostDBProfile updates the provided setting of the hostdb profile with the provided
// name to the provided value.
func (hdb *HostDB) ConfigHostDBProfile(name, setting, value string) (err error) {
	// Change setting.
	err = hdb.hostdbProfiles.ConfigHostDBProfiles(name, setting, value)
	if err != nil {
		return
	}

	// Save to persistence data.
	hdb.mu.Lock()
	hdb.saveSync()
	hdb.mu.Unlock()
	return
}

// DeleteHostDBProfile deletes the hostdb profile with the provided name.
func (hdb *HostDB) DeleteHostDBProfile(name string) (err error) {
	// Delete hostdb profile.
	err = hdb.hostdbProfiles.DeleteHostDBProfile(name)
	if err != nil {
		return
	}

	// Save to persistence data.
	hdb.mu.Lock()
	hdb.saveSync()
	hdb.mu.Unlock()
	return
}

// loadHostTrees loads one host tree for each hostdb profile.
// The host tree is used to manage hosts and query them at random.
func (hdb *HostDB) loadHostTrees(allHosts []modules.HostDBEntry) (err error) {
	hdbp := hdb.hostdbProfiles.HostDBProfiles()
	for name := range hdbp {
		newTree := hosttree.NewHostTree(hdb.calculateHostWeight, name)
		for _, host := range allHosts {
			// COMPATv1.1.0
			//
			// The host did not always track its block height correctly, meaning
			// that previously the FirstSeen values and the blockHeight values
			// could get out of sync.
			if hdb.blockHeight < host.FirstSeen {
				host.FirstSeen = hdb.blockHeight
			}

			err := newTree.Insert(host)
			if err != nil {
				hdb.log.Debugln("ERROR: could not insert host while loading:", host.NetAddress)
			}
		}
		err := hdb.hostTrees.AddHostTree(name, *newTree)

		// Make sure that all hosts have gone through the initial scanning.
		// A new iteration over the tree is necessary as queueScan() which is
		// called below needs a loaded and written back to the variable host tree.
		for _, host := range hdb.hostTrees.All(name) {
			if len(host.ScanHistory) < 2 {
				hdb.mu.Lock()
				hdb.queueScan(host)
				hdb.mu.Unlock()
			}
		}

		hdb.mu.Lock()
		err = hdb.saveSync()
		hdb.mu.Unlock()
		if err != nil {
			hdb.log.Println("Unable to save the host tree:", err)
		}
	}
	return
}

// InitialScanComplete returns a boolean indicating if the initial scan of the
// hostdb is completed.
func (hdb *HostDB) InitialScanComplete() (complete bool, err error) {
	if err = hdb.tg.Add(); err != nil {
		return
	}
	defer hdb.tg.Done()
	hdb.mu.Lock()
	defer hdb.mu.Unlock()
	complete = hdb.initialScanComplete
	return
}

// RandomHosts implements the HostDB interface's RandomHosts() method. It takes
// the tree from which the hosts should be picked, a number of hosts to return,
// and a slice of public keys to exclude, and returns a slice of entries.
func (hdb *HostDB) RandomHosts(tree string, n int, excludeKeys []types.SiaPublicKey) ([]modules.HostDBEntry, error) {
	hdb.mu.RLock()
	initialScanComplete := hdb.initialScanComplete
	hdb.mu.RUnlock()
	if !initialScanComplete {
		return []modules.HostDBEntry{}, ErrInitialScanIncomplete
	}
	return hdb.hostTrees.SelectRandom(tree, n, excludeKeys), nil
}

// UntarGeoLite2 untars the GeoLite2 database downloaded from MaxMind and saves it
// to the geolocationDir.
func UntarGeoLite2(dst string, r io.Reader) error {

	gzr, err := gzip.NewReader(r)
	defer gzr.Close()
	if err != nil {
		return err
	}

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()

		switch {

		// if no more files are found return
		case err == io.EOF:
			return nil

			// return any other error
		case err != nil:
			return err

			// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		// the target location where the dir/file should be created
		var target string
		switch {
		case strings.HasSuffix(header.Name, "/"):
			// Geolocation directory.
			target = filepath.Join(dst, geolocationDir)
		case strings.HasSuffix(header.Name, ".txt"):
			// Copyright and license file.
			filename := strings.Split(header.Name, "/")
			target = filepath.Join(dst, geolocationDir, filename[1])
		case strings.HasSuffix(header.Name, ".mmdb"):
			// Actual GeoLite2 database.
			target = filepath.Join(dst, geolocationDir, geolocationFile)
		}

		// the following switch could also be done using fi.Mode(), not sure if there
		// a benefit of using one vs. the other.
		// fi := header.FileInfo()

		// check the file type
		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}

			// if it's a file create it
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			defer f.Close()

			// copy over contents
			if _, err := io.Copy(f, tr); err != nil {
				return err
			}
		}
	}
}
