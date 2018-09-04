package hostdb

import (
	"path/filepath"
	"time"

	"github.com/pachisi456/Sia/modules"
	"github.com/pachisi456/Sia/modules/renter/hostdb/hostdbprofile"
	"github.com/pachisi456/Sia/persist"
	"github.com/pachisi456/Sia/types"
)

var (
	// persistFilename defines the name of the file that holds the hostdb's
	// persistence.
	persistFilename = "hostdb.json"

	// persistMetadata defines the metadata that tags along with the most recent
	// version of the hostdb persistence file.
	persistMetadata = persist.Metadata{
		Header:  "HostDB Persistence",
		Version: "0.5",
	}
)

// hdbPersist defines what HostDB data persists across sessions.
type hdbPersist struct {
	Profiles    map[string]*hostdbprofile.HostDBProfile
	AllHosts    []modules.HostDBEntry
	BlockHeight types.BlockHeight
	LastChange  modules.ConsensusChangeID
}

// persistData returns the data in the hostdb that will be saved to disk.
func (hdb *HostDB) persistData() (data hdbPersist) {
	data.Profiles = hdb.HostDBProfiles()

	// This is nothing hostdb profile specific so the default host tree can be used.
	data.AllHosts = hdb.hostTrees.All("default")

	data.BlockHeight = hdb.blockHeight
	data.LastChange = hdb.lastChange
	return data
}

// saveSync saves the hostdb persistence data to disk and then syncs to disk.
func (hdb *HostDB) saveSync() error {
	return hdb.deps.SaveFileSync(persistMetadata, hdb.persistData(), filepath.Join(hdb.persistDir, persistFilename))
}

// load loads the hostdb persistence data from disk and returns all hosts found
// in the hostdb persistence data.
func (hdb *HostDB) load() (error, []modules.HostDBEntry) {
	// Fetch the data from the file.
	var data hdbPersist
	err := hdb.deps.LoadFile(persistMetadata, &data, filepath.Join(hdb.persistDir, persistFilename))
	if err != nil {
		return err, nil
	}

	// Set the hostdb internal values.
	if data.Profiles != nil {
		// if no hostdb profile data could be loaded the calling function will add
		// the default profile
		hdb.hostdbProfiles.SetHostDBProfiles(data.Profiles)
	}
	hdb.blockHeight = data.BlockHeight
	hdb.lastChange = data.LastChange
	return nil, data.AllHosts
}

// threadedSaveLoop saves the hostdb to disk every 2 minutes, also saving when
// given the shutdown signal.
func (hdb *HostDB) threadedSaveLoop() {
	for {
		select {
		case <-hdb.tg.StopChan():
			return
		case <-time.After(saveFrequency):
			hdb.mu.Lock()
			err := hdb.saveSync()
			hdb.mu.Unlock()
			if err != nil {
				hdb.log.Println("Difficulties saving the hostdb:", err)
			}
		}
	}
}
