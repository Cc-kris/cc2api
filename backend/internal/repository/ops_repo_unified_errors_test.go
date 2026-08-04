package repository

import (
	"strings"
	"testing"
)

func TestUnifiedErrorSQLClassificationKeepsSpecialCasesAligned(t *testing.T) {
	categorySQL := unifiedErrorCategorySQL()
	subcategorySQL := unifiedErrorSubcategorySQL()
	for name, sql := range map[string]string{
		"category":    categorySQL,
		"subcategory": subcategorySQL,
	} {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(sql, "codex image channels require https responses transport") {
				t.Fatalf("%s SQL does not contain image WS-to-HTTP fallback rule", name)
			}
			if !strings.Contains(sql, "upstream_status_code > 0") {
				t.Fatalf("%s SQL does not use positive upstream status evidence", name)
			}
		})
	}
	if !strings.Contains(unifiedClientSubcategorySQL(), "api_key_quota_exhausted") {
		t.Fatal("client SQL does not prioritize API key quota exhaustion")
	}
}
