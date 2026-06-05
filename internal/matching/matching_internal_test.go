package matching

import "testing"

// White-box tests for the matching package, added in U3. These lock the
// name- and path-based matching behavior the powerhmc validation (U1)
// exercised, so U6/U7 changes to the matching layer surface as deliberate
// test changes rather than silent regressions.

func TestMatchResourceByName_PowerhmcShapes(t *testing.T) {
	resourceNames := map[string]bool{"vios": true, "sys_config": true, "lpar": true}

	cases := []struct {
		funcName string
		want     string
		wantOK   bool
	}{
		{"TestAccViosResource_BasicLifecycle", "vios", true},
		{"TestAccViosResource_ImportInvalidID", "vios", true},
		{"TestAccLparResource_Basic", "lpar", true},
		{"TestAccUnrelatedThing_Basic", "", false},
	}

	for _, tc := range cases {
		got, ok := MatchResourceByName(tc.funcName, resourceNames)
		if ok != tc.wantOK || (ok && got != tc.want) {
			t.Errorf("MatchResourceByName(%q) = (%q, %v); want (%q, %v)", tc.funcName, got, ok, tc.want, tc.wantOK)
		}
	}
}

// Note: ExtractResourceNameFromPath returns (name, isDataSource). A not-found
// result is signaled by an empty name, NOT by the bool. The function only
// matches *_test.go files (non-test source files return "").
func TestExtractResourceNameFromPath_FrameworkConventions(t *testing.T) {
	cases := []struct {
		path     string
		wantName string
		wantIsDS bool
	}{
		{"vios_resource_test.go", "vios", false},
		{"system_config_data_source_test.go", "system_config", true},
		// Non-test source files are not matched (require _test.go).
		{"lpar_resource.go", "", false},
		{"system_config_data_source.go", "", false},
	}

	for _, tc := range cases {
		gotName, gotIsDS := ExtractResourceNameFromPath(tc.path)
		if gotName != tc.wantName {
			t.Errorf("ExtractResourceNameFromPath(%q) name = %q; want %q", tc.path, gotName, tc.wantName)
		}
		if gotName != "" && gotIsDS != tc.wantIsDS {
			t.Errorf("ExtractResourceNameFromPath(%q) isDataSource = %v; want %v", tc.path, gotIsDS, tc.wantIsDS)
		}
	}
}
