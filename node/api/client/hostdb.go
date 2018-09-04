package client

import (
	"net/url"

	"github.com/pachisi456/Sia/node/api"
	"github.com/pachisi456/Sia/types"
	"github.com/pachisi456/Sia/modules/renter/hostdb/hostdbprofile"
	"strings"
)

// HostDbGet requests the /hostdb endpoint's resources.
func (c *Client) HostDbGet() (hdg api.HostdbGet, err error) {
	err = c.get("/hostdb", &hdg)
	return
}

// HostDbActiveGet requests the /hostdb/active endpoint's resources.
func (c *Client) HostDbActiveGet() (hdag api.HostdbActiveGET, err error) {
	err = c.get("/hostdb/active", &hdag)
	return
}

// HostDbAllGet requests the /hostdb/all endpoint's resources.
func (c *Client) HostDbAllGet() (hdag api.HostdbAllGET, err error) {
	err = c.get("/hostdb/all", &hdag)
	return
}

// HostDbHostsGet request the /hostdb/hosts/:pubkey endpoint's resources.
func (c *Client) HostDbHostsGet(pk types.SiaPublicKey) (hhg api.HostdbHostsGET, err error) {
	err = c.get("/hostdb/hosts/"+pk.String(), &hhg)
	return
}

// HostDbProfilesGet requests the /hostdb/profiles endpoint's resources.
func (c *Client) HostDbProfilesGet() (hdbp map[string]*hostdbprofile.HostDBProfile, err error) {
	err = c.get("/hostdb/profiles", &hdbp)
	return
}

// HostDbProfilesAddPost posts a new profile to add to the hostdb profiles
// API route /hostdb/profiles/add
func (c *Client) HostDbProfilesAddPost(name, storagetier string) (err error) {
	values := url.Values{}
	values.Set("name", strings.ToLower(name))
	values.Set("storagetier", strings.ToLower(storagetier))
	err = c.post("/hostdb/profiles/add", values.Encode(), nil)
	return
}

// HostDbProfilesConfigPost posts a config to a hostdb profile. API route
// /hostdb/profiles/config
func (c *Client) HostDbProfilesConfigPost(name, setting, value string) (err error) {
	values := url.Values{}
	values.Set("name", strings.ToLower(name))
	values.Set("setting", strings.ToLower(setting))
	values.Set("value", strings.ToLower(value))
	err = c.post("/hostdb/profiles/config", values.Encode(), nil)
	return
}

// HostDbProfilesDeletePost makes a post request to delete the hostdb profile with
// the provided name.
func (c *Client) HostDbProfilesDeletePost(name string) (err error) {
	values := url.Values{}
	values.Set("name", strings.ToLower(name))
	err = c.post("/hostdb/profiles/delete", values.Encode(), nil)
	return
}
