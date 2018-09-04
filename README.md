# Customizable Host Selection for Sia

In the context of a bachelor thesis the [official Sia
implementation](https://github.com/NebulousLabs/Sia) is altered to
support customizability of the host selection process.

This client implements so-called hostdb profiles, for which the user
can set a list of locations within which they wish their storage to
reside in. Moreover they can specify a storage tier (as "cold", "warm"
or "hot" storage). The location feature blacklists all hosts which due
to their IP address appear to be from a country not whitelisted. The
storage tier puts different weights on host prices when scoring hosts
(cold storage puts more weight on storage price, hot storage puts
more weight on bandwidth (up- and download) prices, warm storage (which
is the default setting) weighs all prices equally, as in the official
implementation).

## Installation

* [Install Golang](https://golang.org/dl/)
* run `go get -u github.com/pachisi456/Sia/...`
* In the cloned repository run `make`
* In Gobin execute `siad` (starts the Sia Daemon)
* Open another terminal and run `siac` and start playing around with it
* Explore the command `siac hostdb profiles` to play around with hostdb profiles
