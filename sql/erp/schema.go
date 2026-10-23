// Package erpsql contains reviewed schema assets only; it never opens a database.
package erpsql

import _ "embed"

// Phase1 is shared by the explicit installer and read-only startup verifier.
//
//go:embed 003_order_fulfillment.sql
var Phase1 string

// Purchasing is an independent immutable version, not an extension of Phase1.
//
//go:embed 004_purchasing.sql
var Purchasing string

// PurchaseReturns is an independent optional version; never auto-installed.
//
//go:embed 005_purchase_returns.sql
var PurchaseReturns string
