package hosttree

import (
	"sync"
	"github.com/pachisi456/Sia/modules"
	"github.com/pachisi456/Sia/types"
)

// HostTrees holds the map of all host trees (one tree for each hostdb profile)
// mapped by the respective hostdb profile name.
type HostTrees struct {
	trees map[string]*HostTree
	mu    sync.Mutex
}

// NewHostTrees creates a new, empty HostTrees object.
func NewHostTrees() HostTrees {
	ht := HostTrees{
		trees: make(map[string]*HostTree),
	}
	return ht
}

// AddHostTree adds a host tree to the map of trees at the given name.
func (ht *HostTrees) AddHostTree(name string, tree HostTree) error {
	ht.mu.Lock()
	defer ht.mu.Unlock()
	if _, exists := ht.trees[name]; exists {
		return errTreeExists
	}
	ht.trees[name] = &tree
	return nil
}

// All returns all of the hosts in the host tree with the provided name, sorted by weight.
func (ht *HostTrees) All(tree string) []modules.HostDBEntry {
	ht.mu.Lock()
	defer ht.mu.Unlock()
	return ht.trees[tree].All()
}

// Insert inserts the entry provided to `entry` into all existing host trees. Insert will
// return an error if the input host already exists.
// ht needs to be locked when using Insert.
func (ht *HostTrees) Insert(hdbe modules.HostDBEntry) error {
	ht.mu.Lock()
	defer ht.mu.Unlock()
	for _, tree := range ht.trees {
		err := tree.Insert(hdbe)
		if err != nil {
			return err
		}
	}
	return nil
}

// Modify updates a host entry at the given public key, replacing the old entry
// in each of the host trees.
func (ht *HostTrees) Modify(hdbe modules.HostDBEntry) error {
	ht.mu.Lock()
	defer ht.mu.Unlock()
	for _, tree := range ht.trees {
		err := tree.Modify(hdbe)
		if err != nil {
			return err
		}
	}
	return nil
}

// Remove removes the host with the public key provided by `pk` from all trees.
func (ht *HostTrees) Remove(pk types.SiaPublicKey) error {
	ht.mu.Lock()
	defer ht.mu.Unlock()
	for _, tree := range ht.trees {
		err := tree.Remove(pk)
		if err != nil {
			return err
		}
	}
	return nil
}

// Select returns the host with the provided public key, should the host exist.
func (ht *HostTrees) Select(spk types.SiaPublicKey) (modules.HostDBEntry, bool) {
	ht.mu.Lock()
	defer ht.mu.Unlock()
	// This is nothing hostdb profile specific so the default host tree can be used.
	return ht.trees["default"].Select(spk)
}

// SelectRandom grabs a random n hosts from the provided tree. There will be no repeats,
// but the length of the slice returned may be less than n, and may even be zero.
// The hosts that are returned first have the higher priority. Hosts passed to
// 'ignore' will not be considered; pass `nil` if no blacklist is desired.
func (ht *HostTrees) SelectRandom(tree string, n int, ignore []types.SiaPublicKey) []modules.HostDBEntry {
	ht.mu.Lock()
	defer ht.mu.Unlock()
	return ht.trees[tree].SelectRandom(n, ignore)
}
