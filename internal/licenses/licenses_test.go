package licenses

import (
	"strings"
	"testing"
)

func TestList(t *testing.T) {
	licenses, err := List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(licenses) == 0 {
		t.Fatal("List() returned empty slice, expected licenses")
	}

	// Verify licenses are sorted by package name
	for i := 1; i < len(licenses); i++ {
		if licenses[i-1].Package >= licenses[i].Package {
			t.Errorf("licenses not sorted: %s >= %s", licenses[i-1].Package, licenses[i].Package)
		}
	}

	// Check that each license has required fields
	for _, lic := range licenses {
		if lic.Package == "" {
			t.Error("found license with empty package")
		}
		if lic.Type == "" {
			t.Errorf("license %s has empty type", lic.Package)
		}
	}
}

func TestGetLicenseText(t *testing.T) {
	licenses, err := List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(licenses) == 0 {
		t.Skip("no licenses available")
	}

	// Try to get license text for first few packages
	found := 0
	for _, lic := range licenses {
		text, err := GetLicenseText(lic.Package)
		if err == nil {
			found++
			if text == "" {
				t.Errorf("GetLicenseText(%s) returned empty text", lic.Package)
			}
			// Most license files contain "Copyright" or "LICENSE" or "MIT" etc
			textLower := strings.ToLower(text)
			if !strings.Contains(textLower, "copyright") &&
				!strings.Contains(textLower, "license") &&
				!strings.Contains(textLower, "permission") {
				t.Errorf("GetLicenseText(%s) returned text without expected keywords", lic.Package)
			}
		}
	}

	if found == 0 {
		t.Error("could not find any embedded license texts")
	}
}

func TestGetLicenseText_NotFound(t *testing.T) {
	_, err := GetLicenseText("nonexistent/package")
	if err == nil {
		t.Error("GetLicenseText(nonexistent) should return error")
	}
}

func TestGetAllLicenseTexts(t *testing.T) {
	text, err := GetAllLicenseTexts()
	if err != nil {
		t.Fatalf("GetAllLicenseTexts() error = %v", err)
	}

	if text == "" {
		t.Error("GetAllLicenseTexts() returned empty text")
	}

	// Should contain section headers
	if !strings.Contains(text, "Package:") {
		t.Error("GetAllLicenseTexts() missing Package: headers")
	}
	if !strings.Contains(text, "License:") {
		t.Error("GetAllLicenseTexts() missing License: headers")
	}
}

func TestCount(t *testing.T) {
	count := Count()
	if count == 0 {
		t.Error("Count() returned 0, expected positive number")
	}

	// Verify count matches List()
	licenses, err := List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if count != len(licenses) {
		t.Errorf("Count() = %d, want %d", count, len(licenses))
	}
}

func TestLicenseTypes(t *testing.T) {
	types := LicenseTypes()
	if len(types) == 0 {
		t.Error("LicenseTypes() returned empty map")
	}

	// Verify total count matches
	total := 0
	for _, count := range types {
		total += count
	}

	licenses, err := List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if total != len(licenses) {
		t.Errorf("LicenseTypes() total = %d, want %d", total, len(licenses))
	}
}

func TestEmbeddedLicenseCount(t *testing.T) {
	count := EmbeddedLicenseCount()
	// We should have at least some embedded licenses
	if count == 0 {
		t.Error("EmbeddedLicenseCount() returned 0, expected positive number")
	}
}

func TestLicenseCSV_NotEmpty(t *testing.T) {
	if len(LicensesCSV) == 0 {
		t.Error("LicensesCSV is empty")
	}
}
