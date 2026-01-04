package licenses

import (
	"encoding/csv"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// License represents a third-party dependency's license information.
type License struct {
	Package string // Full package path (e.g., "github.com/spf13/cobra")
	URL     string // URL to the license file
	Type    string // License type (e.g., "MIT", "BSD-3-Clause")
}

// List returns all third-party licenses parsed from the embedded CSV.
func List() ([]License, error) {
	reader := csv.NewReader(strings.NewReader(string(LicensesCSV)))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse licenses CSV: %w", err)
	}

	licenses := make([]License, 0, len(records))
	for _, record := range records {
		if len(record) < 3 {
			continue
		}
		licenses = append(licenses, License{
			Package: record[0],
			URL:     record[1],
			Type:    record[2],
		})
	}

	// Sort by package name for consistent output
	sort.Slice(licenses, func(i, j int) bool {
		return licenses[i].Package < licenses[j].Package
	})

	return licenses, nil
}

// GetLicenseText returns the full license text for a specific package.
// The package path should match exactly as it appears in the CSV.
func GetLicenseText(packagePath string) (string, error) {
	// Try common license file names
	licenseNames := []string{"LICENSE", "LICENSE.txt", "LICENSE.md", "LICENCE", "COPYING"}

	for _, name := range licenseNames {
		path := filepath.Join("third_party", packagePath, name)
		data, err := ThirdPartyFS.ReadFile(path)
		if err == nil {
			return string(data), nil
		}
	}

	return "", fmt.Errorf("license file not found for package: %s", packagePath)
}

// GetAllLicenseTexts returns all license texts concatenated together.
// Each license is prefixed with a header showing the package name.
func GetAllLicenseTexts() (string, error) {
	licenses, err := List()
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	for _, lic := range licenses {
		text, err := GetLicenseText(lic.Package)
		if err != nil {
			// Skip packages without embedded license text
			continue
		}

		sb.WriteString("=" + strings.Repeat("=", 78) + "\n")
		sb.WriteString(fmt.Sprintf("Package: %s\n", lic.Package))
		sb.WriteString(fmt.Sprintf("License: %s\n", lic.Type))
		sb.WriteString("=" + strings.Repeat("=", 78) + "\n\n")
		sb.WriteString(text)
		sb.WriteString("\n\n")
	}

	return sb.String(), nil
}

// Count returns the number of third-party dependencies.
func Count() int {
	licenses, err := List()
	if err != nil {
		return 0
	}
	return len(licenses)
}

// LicenseTypes returns a map of license types to their counts.
func LicenseTypes() map[string]int {
	licenses, err := List()
	if err != nil {
		return nil
	}

	types := make(map[string]int)
	for _, lic := range licenses {
		types[lic.Type]++
	}
	return types
}

// EmbeddedLicenseCount returns the number of packages with embedded license texts.
func EmbeddedLicenseCount() int {
	count := 0
	err := fs.WalkDir(ThirdPartyFS, "third_party", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		name := filepath.Base(path)
		if strings.HasPrefix(strings.ToUpper(name), "LICENSE") ||
			strings.HasPrefix(strings.ToUpper(name), "LICENCE") ||
			name == "COPYING" {
			count++
		}
		return nil
	})
	if err != nil {
		return 0
	}
	return count
}
