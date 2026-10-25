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

// SalesReturns is a separate CREATE-only optional version.
//
//go:embed 006_sales_returns.sql
var SalesReturns string

// WarehouseOperations is an independent CREATE-only optional version.
//
//go:embed 007_warehouse_operations.sql
var WarehouseOperations string

// Finance is explicit CREATE-only, never installed by server startup.
//
//go:embed 008_finance.sql
var Finance string

//go:embed 009_sales_finance.sql
var SalesFinance string

// Catalog is optional CREATE-only; startup only verifies.
//
//go:embed 010_catalog.sql
var Catalog string

// Logistics is explicit CREATE-only; existing package fields are untouched.
//
//go:embed 011_logistics.sql
var Logistics string
