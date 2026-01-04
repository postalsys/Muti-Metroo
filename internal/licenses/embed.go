// Package licenses provides embedded third-party license information.
package licenses

import "embed"

// ThirdPartyFS contains the embedded license files for all third-party dependencies.
//
//go:embed third_party/*
var ThirdPartyFS embed.FS

// LicensesCSV contains the CSV report of all dependencies with their license types.
//
//go:embed licenses.csv
var LicensesCSV []byte
