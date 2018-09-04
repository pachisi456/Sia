package hostdbprofile

import (
	"errors"
	"sync"
)

var (
	errHostdbProfileExists = errors.New("hostdb profile with provided name already exists")
	errLocationNotSet      = errors.New("provided location cannot be removed as it is not set")
	errLocationAlreadySet  = errors.New("provided location is already set")
	errNoSuchHostdbProfile = errors.New("hostdb profile with provided name does not exist")
	errNoSuchLocation      = errors.New("provided location not recognized")
	errNoSuchSetting       = errors.New("provided setting not recognized")
	errNoSuchStorageTier   = errors.New("no such storage tier, see `siac hostdb profiles add " +
		"-h` for possible storage tiers")
	errStoragetierAlreadySet = errors.New("provided storage tier is already set")
)

// HostDBProfiles is the collection of all hostdb profiles the renter created to
// customize the host selection. The profiles are mapped by the name given by the user.
type HostDBProfiles struct {
	profiles map[string]*HostDBProfile
	mu       sync.Mutex
}

// NewHostDBProfiles creates a new HostDBProfiles object and initializes it with the
// default hostdb profile.
func NewHostDBProfiles() HostDBProfiles {
	hdbp := make(map[string]*HostDBProfile)
	hdbp["default"] = &HostDBProfile{
		Storagetier: "warm",
		Location:    nil,
	}
	return HostDBProfiles{
		profiles: hdbp,
	}
}

// AddHostDBProfile adds a new hostdb profile to HostDBProfiles.
func (hdbp *HostDBProfiles) AddHostDBProfile(name, storagetier string) (err error) {
	hdbp.mu.Lock()
	defer hdbp.mu.Unlock()

	// check if hostdb profile with that name already exists
	if _, exists := hdbp.profiles[name]; exists {
		return errHostdbProfileExists
	}

	// check if provided storage tier is valid
	if valid := storagetierValid(storagetier); !valid {
		return errNoSuchStorageTier
	}

	// add new hostdb profile
	hdbp.profiles[name] = &HostDBProfile{
		Storagetier: storagetier,
		Location:    nil,
	}
	return
}

// ConfigHostDBProfiles updates the provided setting of the hostdb profile with the provided
// name to the provided value.
func (hdbp *HostDBProfiles) ConfigHostDBProfiles(name, setting, value string) error {
	hdbp.mu.Lock()
	defer hdbp.mu.Unlock()

	// check if profile exists
	if _, exists := hdbp.profiles[name]; !exists {
		return errNoSuchHostdbProfile
	}

	return hdbp.profiles[name].configHostDBProfile(setting, value)
}

// DeleteHostDBProfile deletes the hostdb profile with the provided name.
func (hdbp *HostDBProfiles) DeleteHostDBProfile(name string) error {
	hdbp.mu.Lock()
	defer hdbp.mu.Unlock()

	// check if profile exists
	if _, exists := hdbp.profiles[name]; !exists {
		return errNoSuchHostdbProfile
	}

	delete(hdbp.profiles, name)
	return nil
}

// getProfile returns the hostdb profile with the given name.
func (hdbp *HostDBProfiles) GetProfile(name string) HostDBProfile {
	hdbp.mu.Lock()
	defer hdbp.mu.Unlock()
	return *hdbp.profiles[name]
}

// HostDBProfiles returns the array of set hostdb profiles.
func (hdbp *HostDBProfiles) HostDBProfiles() map[string]*HostDBProfile {
	hdbp.mu.Lock()
	defer hdbp.mu.Unlock()
	return hdbp.profiles
}

// SetHostDBProfiles sets the hostdb profiles to the profiles passed to the function (from persist data)
func (hdbp *HostDBProfiles) SetHostDBProfiles(profiles map[string]*HostDBProfile) {
	hdbp.profiles = profiles
	return
}

// storageTierValid is a helper function that returns true if the provided storage tier is valid,
// otherwise false.
func storagetierValid(storagetier string) (valid bool) {
	for _, v := range storagetiers {
		if v == storagetier {
			return true
		}
	}
	return
}

// locationValid is helper a function that returns true if the provided location is valid,
// otherwise false.
func locationValid(location string) (valid bool) {
	for _, v := range locations {
		if v == location {
			return true
		}
	}
	return
}
