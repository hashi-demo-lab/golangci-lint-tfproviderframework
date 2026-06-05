package fileutil

import "testing"

func TestSnakeCase(t *testing.T) {
	cases := map[string]string{
		"WidgetResource": "widget_resource",
		"HTTPServer":     "http_server",
		"systemConfig":   "system_config",
		"":               "",
	}
	for in, want := range cases {
		if got := SnakeCase(in); got != want {
			t.Errorf("SnakeCase(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestTitleCase(t *testing.T) {
	cases := map[string]string{
		"sys_config": "SysConfig",
		"widget":     "Widget",
	}
	for in, want := range cases {
		if got := TitleCase(in); got != want {
			t.Errorf("TitleCase(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestExtractResourceName(t *testing.T) {
	cases := map[string]string{
		"WidgetResource": "widget",
		"HttpDataSource": "http",
		"JobAction":      "job",
	}
	for in, want := range cases {
		if got := ExtractResourceName(in); got != want {
			t.Errorf("ExtractResourceName(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestClassifiers(t *testing.T) {
	if !IsBaseClassFile("base_widget.go") || IsBaseClassFile("resource_widget.go") {
		t.Error("IsBaseClassFile")
	}
	if !IsSweeperFile("compute_sweeper.go") || IsSweeperFile("resource_widget.go") {
		t.Error("IsSweeperFile")
	}
	if !IsMigrationFile("resource_state_upgrader.go") || IsMigrationFile("resource_widget.go") {
		t.Error("IsMigrationFile")
	}
}

func TestShouldExcludeFile(t *testing.T) {
	if !ShouldExcludeFile("a/b_sweeper.go", []string{"*_sweeper.go"}) {
		t.Error("expected exclude by base-name glob")
	}
	if ShouldExcludeFile("a/resource.go", []string{"*_sweeper.go"}) {
		t.Error("did not expect exclude")
	}
}
