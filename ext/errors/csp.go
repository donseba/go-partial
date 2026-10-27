package errors

import (
	_ "embed"
	"io/fs"

	"github.com/donseba/go-partial/internal/asset"
	"github.com/donseba/go-partial/internal/csp"
)

// stylesheet styles the classes of the built-in error fragment; see
// StylesheetFS.
//
//go:embed fragment.css
var stylesheet []byte

// StylesheetName is the name of the stylesheet in StylesheetFS.
const StylesheetName = "partial-errors.css"

// StyleHash returns the Content-Security-Policy hash source of the inline
// <style> element of the built-in error page, 'sha256-…' with its quotes,
// for a policy without 'unsafe-inline':
//
//	style-src 'self' 'sha256-…'
//
// Add it to the policy of the error page, and to that of every page whose
// links or forms carry data-partial-reload-on-error: the interactions
// client writes the error page of a failed POST into the page that sent
// it, under that page's policy. The hash changes only when go-partial
// changes the page's style; an application that answers with an error
// template of its own (WithTemplate) needs none. The built-in error
// fragment has no inline style; see StylesheetFS.
func StyleHash() string {
	return csp.Hash(pageStyle)
}

// StylesheetFS returns a file system that holds the CSS of the built-in
// error fragment as StylesheetName, for applications that serve,
// fingerprint and version it with their other assets. The fragment takes
// the place of a failing part of a page and answers a failing partial
// request inside the application's own page. It carries no style of its
// own, so that a Content-Security-Policy without 'unsafe-inline' lets it
// through: serve this file and link it from the application's layout, copy
// its rules into the application's CSS, or style the classes
// go-partial-error, go-partial-error__title, go-partial-error__message and
// go-partial-error__details differently. Unstyled, the fragment reads as a
// heading, a paragraph and a list.
//
//	assets := partial.Overlay(appAssets, errors.StylesheetFS())
//
//	<link rel="stylesheet" href="/assets/partial-errors.css">
func StylesheetFS() fs.FS {
	return asset.FS(map[string][]byte{StylesheetName: stylesheet})
}
