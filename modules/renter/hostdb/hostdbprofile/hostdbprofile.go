package hostdbprofile

var (
	// storagetiers is an array of all possible storage tiers that the user can
	// choose between when creating a new hostdb profile. Depending on the
	// "temperature" set here less performant but cheaper hosts (cold) or more
	// performant but more expensive hosts (hot) are picked in the host selection.
	// Warm is balanced between hot and cold and thus the default setting.
	storagetiers = []string{"cold", "warm", "hot"}

	// locations is the list of possible locations the user can restrict their hostdb
	// profile to. Siad will then only form contracts with hosts in those locations
	// (according to ip address)
	//TODO pachisi456: complete this list (should contain all countries/regions of the world)
	locations = []string{"eu", "germany", "china", "united states", "russia"}
)

// HostDBProfile is a hostdb profile for customizable settings concerning the
// selection of hosts.
type HostDBProfile struct {
	Storagetier string   `json:"storagetier"`
	Location    []string `json:"location"`
}

// configHostDBProfile updates the provided setting of a hostdb profile to the provided value.
// All parameters are checked for validity.
func (hdbp *HostDBProfile) configHostDBProfile(setting, value string) (err error) {
	switch setting {
	case "storagetier":
		// check if provided storage tier is not set already
		if hdbp.Storagetier == value {
			return errStoragetierAlreadySet
		}
		// check if provided storage tier is valid
		if valid := storagetierValid(value); !valid {
			return errNoSuchStorageTier
		}
		// adjust the storage tier
		hdbp.Storagetier = value
	case "addlocation":
		// check if location is already set
		for _, l := range hdbp.Location {
			if l == value {
				return errLocationAlreadySet
			}
		}
		// check if provided location is valid
		if valid := locationValid(value); !valid {
			return errNoSuchLocation
		}
		// add location
		hdbp.Location = append(hdbp.Location, value)
	case "removelocation":
		// check if and at what index the provided location is set
		index := -1
		for i, location := range hdbp.Location {
			if location == value {
				index = i
				break
			}
		}

		// return error if location not found
		if index < 0 {
			return errLocationNotSet
		}

		// delete the location
		hdbp.Location = append(hdbp.Location[:index], hdbp.Location[index+1:]...)
	default:
		return errNoSuchSetting
	}

	return
}
