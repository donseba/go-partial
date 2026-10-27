// Package clients holds go-partial's browser clients in one file system,
// for applications that serve, fingerprint and version them with their
// other assets, such as through an import map:
//
//	partial-flash.js         exp/flash's client (flash.ClientScript)
//	partial-sse.js           exp/sse's client (sse.ClientScript)
//	partial-interactions.js  exp/interactions' client (interactions.ClientScript)
//
// All lists them with the names scripts import them by, for an import map.
package clients

import (
	"io/fs"

	"github.com/donseba/go-partial/exp/flash"
	"github.com/donseba/go-partial/exp/interactions"
	"github.com/donseba/go-partial/exp/sse"
	"github.com/donseba/go-partial/internal/asset"
)

// Client is one of go-partial's browser clients: the name an import map
// gives it and its file in FS.
type Client struct {
	// Import is the module name scripts import it by, such as
	// "partial-flash" for import "partial-flash".
	Import string
	// File is its name in FS, the package's ClientName, such as
	// "partial-flash.js".
	File string
}

// All returns go-partial's browser clients, for an application that maps
// each in its import map:
//
//	for _, client := range clients.All() {
//		importMap.WithFile(client.Import, client.File)
//	}
func All() []Client {
	return []Client{
		{Import: "partial-flash", File: flash.ClientName},
		{Import: "partial-sse", File: sse.ClientName},
		{Import: "partial-interactions", File: interactions.ClientName},
	}
}

// FS returns the browser clients of go-partial, each under its ClientName.
//
//	assets := partial.Overlay(appAssets, clients.FS())
func FS() fs.FS {
	return asset.FS(map[string][]byte{
		flash.ClientName:        flash.ClientScript(),
		sse.ClientName:          sse.ClientScript(),
		interactions.ClientName: interactions.ClientScript(),
	})
}
