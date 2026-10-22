// Package erpsql contains reviewed schema assets only; it never opens a database.
package erpsql

import _ "embed"

// Phase1 is shared by the explicit installer and read-only startup verifier.
//
//go:embed 003_order_fulfillment.sql
var Phase1 string
