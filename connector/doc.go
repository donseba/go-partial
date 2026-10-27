// Package connector tells go-partial how a client library makes its
// requests and reads its responses: htmx 2 (NewHTMX), htmx 4 (NewHTMX4),
// Unpoly (NewUnpoly), Turbo (NewTurbo), or plain fetch and tests
// (NewPartial). A connector reads the target, selection and action a request
// asks for, and writes response instructions, interaction and link
// attributes, out-of-band markers and redirects in the library's terms.
package connector
