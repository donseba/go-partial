// Package partial provides request-aware rendering for Go templates.
//
// Partial trees, Stages, connectors, and template caches
// are safe to reuse across concurrent renders after configuration is complete.
// Request-scoped state is carried by RenderContext and is not stored on the
// reusable Partial configuration.
//
// Configuration methods such as SetFunc, Use, With, SetDot, SetFileSystem,
// SetConnector, SetResponseHeaders, and extension setup helpers mutate the
// partial they are called on. Call them before serving requests, or
// synchronize access externally when changing configuration at runtime.
package partial
