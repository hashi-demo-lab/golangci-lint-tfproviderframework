// Package tfprovidertest implements a golangci-lint plugin that identifies test coverage gaps
// in Terraform providers built with terraform-plugin-framework.
package tfprovidertest

import (
	"testing"

	"github.com/example/tfprovidertest/internal/matching"
	"github.com/example/tfprovidertest/internal/registry"
)

func TestExtractResourceFromFuncName(t *testing.T) {
	tests := []struct {
		funcName     string
		wantResource string
		wantFound    bool
	}{
		// Standard patterns
		{"TestAccWidget_basic", "widget", true},
		{"TestAccWidget_update", "widget", true},
		{"TestAccWidget_disappears", "widget", true},

		// Provider prefix patterns - note: the function extracts the full resource name after TestAcc
		// The provider prefix separation happens in ExtractProviderFromFuncName
		{"TestAccAWSInstance_basic", "aws_instance", true},
		{"TestAccGoogleComputeInstance_update", "compute_instance", true},
		{"TestAccAzureRMVirtualMachine_disappears", "rm_virtual_machine", true},

		// Data source patterns
		{"TestAccDataSourceHTTP_basic", "http", true},
		{"TestAccDataSourceAWSAMI_filter", "awsami", true},
		{"TestAccDataSourceWidget_basic", "widget", true},

		// Resource prefix patterns
		{"TestAccResourceWidget_basic", "widget", true},
		{"TestAccResourceServer_update", "server", true},

		// Short provider prefixes (edge cases) - full name is extracted
		{"TestAccS3Bucket_basic", "s3_bucket", true},
		{"TestAccEC2Instance_basic", "ec2_instance", true},
		{"TestAccIAMRole_basic", "iam_role", true},

		// Multi-word resources (CamelCase to snake_case)
		{"TestAccComputeInstance_basic", "instance", true},
		{"TestAccServerGroup_update", "group", true},

		// Non-matching patterns (should return false)
		{"TestHelper", "", false},
		{"TestUnit_something", "", false},
		{"BenchmarkWidget", "", false},
		{"testAccWidget_basic", "", false}, // lowercase 't'
		{"TestWidgetBasic", "", false},     // missing underscore

		// Edge cases
		{"TestAcc_basic", "", false},    // no resource name
		{"TestAccA_basic", "", false},   // single uppercase - doesn't match pattern
		{"TestAccAB_basic", "ab", true}, // two chars get extracted as full name
	}

	for _, tt := range tests {
		t.Run(tt.funcName, func(t *testing.T) {
			got, found := matching.ExtractResourceFromFuncName(tt.funcName)
			if found != tt.wantFound {
				t.Errorf("ExtractResourceFromFuncName(%q) found = %v, want %v", tt.funcName, found, tt.wantFound)
			}
			if got != tt.wantResource {
				t.Errorf("ExtractResourceFromFuncName(%q) resource = %q, want %q", tt.funcName, got, tt.wantResource)
			}
		})
	}
}

func TestExtractProviderFromFuncName(t *testing.T) {
	tests := []struct {
		funcName     string
		wantProvider string
	}{
		// Provider prefixes - the regex looks for uppercase letter followed by lowercase
		{"TestAccAWSInstance_basic", ""}, // AWS is all uppercase - doesn't match provider pattern
		{"TestAccGoogleComputeInstance_update", "google"},
		{"TestAccAzureRMVirtualMachine_disappears", "azure"},

		// No provider prefix
		{"TestAccWidget_basic", ""},
		{"TestAccDataSourceHTTP_basic", "data"},     // "Data" matches provider pattern
		{"TestAccResourceWidget_basic", "resource"}, // "Resource" matches provider pattern

		// Short provider prefixes - pattern requires uppercase + lowercase
		{"TestAccS3Bucket_basic", ""},    // "S" alone doesn't match
		{"TestAccEC2Instance_basic", ""}, // "EC" doesn't match

		// Edge cases
		{"TestHelper", ""},
		{"testAccWidget_basic", ""},
		{"TestAccA_basic", ""},
	}

	for _, tt := range tests {
		t.Run(tt.funcName, func(t *testing.T) {
			got := matching.ExtractProviderFromFuncName(tt.funcName)
			if got != tt.wantProvider {
				t.Errorf("ExtractProviderFromFuncName(%q) = %q, want %q", tt.funcName, got, tt.wantProvider)
			}
		})
	}
}

func TestToSnakeCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"MyResource", "my_resource"},
		{"HTTPServer", "http_server"},
		{"Widget", "widget"},
		{"ComputeInstance", "compute_instance"},
		{"S3Bucket", "s3_bucket"},
		{"IAMRole", "iam_role"},
		{"VirtualMachine", "virtual_machine"},
		{"", ""},
		{"abc", "abc"},
		{"ABC", "abc"},
		{"AWSInstance", "aws_instance"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := matching.CamelCaseToSnakeCaseExported(tt.input)
			if got != tt.expected {
				t.Errorf("toSnakeCase(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestToTitleCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"my_resource", "MyResource"},
		{"widget", "Widget"},
		{"compute_instance", "ComputeInstance"},
		{"s3_bucket", "S3Bucket"},
		{"iam_role", "IamRole"},
		{"", ""},
		{"abc", "Abc"},
		{"a_b_c", "ABC"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := matching.SnakeCaseToTitleCaseExported(tt.input)
			if got != tt.expected {
				t.Errorf("toTitleCase(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestIsSweeperFile(t *testing.T) {
	tests := []struct {
		filePath string
		expected bool
	}{
		{"/path/to/resource_widget_sweeper.go", true},
		{"/path/to/sweeper.go", false}, // doesn't have _ before sweeper
		{"/path/to/aws_sweeper.go", true},
		{"/path/to/resource_widget.go", false},
		{"/path/to/resource_widget_test.go", false},
		{"sweeper.go", false},
		{"my_sweeper.go", true},
	}

	for _, tt := range tests {
		t.Run(tt.filePath, func(t *testing.T) {
			got := matching.IsSweeperFile(tt.filePath)
			if got != tt.expected {
				t.Errorf("IsSweeperFile(%q) = %v, want %v", tt.filePath, got, tt.expected)
			}
		})
	}
}

func TestIsMigrationFile(t *testing.T) {
	tests := []struct {
		filePath string
		expected bool
	}{
		{"/path/to/resource_widget_migrate.go", true},
		{"/path/to/resource_widget_migration.go", true},
		{"/path/to/resource_widget_migration_v1.go", true},
		{"/path/to/resource_widget_state_upgrader.go", true},
		{"/path/to/resource_widget.go", false},
		{"/path/to/resource_widget_test.go", false},
		{"migrate.go", false}, // doesn't have _ before migrate
	}

	for _, tt := range tests {
		t.Run(tt.filePath, func(t *testing.T) {
			got := matching.IsMigrationFile(tt.filePath)
			if got != tt.expected {
				t.Errorf("IsMigrationFile(%q) = %v, want %v", tt.filePath, got, tt.expected)
			}
		})
	}
}

func TestIsBaseClassFile(t *testing.T) {
	tests := []struct {
		filePath string
		expected bool
	}{
		{"/path/to/base_resource.go", true},
		{"/path/to/base.go", true},
		{"/path/to/resource_widget.go", false},
		{"/path/to/database_resource.go", false},
		{"base_test.go", true},
		{"base.go", true},
	}

	for _, tt := range tests {
		t.Run(tt.filePath, func(t *testing.T) {
			got := matching.IsBaseClassFile(tt.filePath)
			if got != tt.expected {
				t.Errorf("isBaseClassFile(%q) = %v, want %v", tt.filePath, got, tt.expected)
			}
		})
	}
}

func TestShouldExcludeFile(t *testing.T) {
	tests := []struct {
		filePath     string
		excludePaths []string
		expected     bool
	}{
		{"/path/to/resource_widget.go", []string{"vendor/"}, false},
		{"/path/to/vendor/resource_widget.go", []string{"vendor/"}, true},
		{"/path/to/resource_widget.go", []string{"*.pb.go"}, false},
		{"/path/to/resource_widget.pb.go", []string{"*.pb.go"}, true},
		{"/path/to/resource_widget.go", []string{}, false},
		{"/path/to/resource_widget.go", []string{"resource_*.go"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.filePath, func(t *testing.T) {
			got := matching.ShouldExcludeFileExported(tt.filePath, tt.excludePaths)
			if got != tt.expected {
				t.Errorf("shouldExcludeFile(%q, %v) = %v, want %v", tt.filePath, tt.excludePaths, got, tt.expected)
			}
		})
	}
}

func TestIsTestFunction(t *testing.T) {
	tests := []struct {
		funcName       string
		customPatterns []string
		expected       bool
	}{
		// Default behavior (no custom patterns)
		{"TestAccWidget_basic", nil, true},
		{"TestWidget", nil, true},
		{"Test_something", nil, true},
		{"testAccWidget_basic", nil, false}, // lowercase
		{"BenchmarkWidget", nil, false},
		{"ExampleWidget", nil, false},

		// With custom patterns
		{"TestAccWidget_basic", []string{"TestAcc"}, true},
		{"TestWidget", []string{"TestAcc"}, false},
		{"TestResourceWidget", []string{"TestResource"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.funcName, func(t *testing.T) {
			got := matching.IsTestFunctionExported(tt.funcName, tt.customPatterns)
			if got != tt.expected {
				t.Errorf("isTestFunction(%q, %v) = %v, want %v", tt.funcName, tt.customPatterns, got, tt.expected)
			}
		})
	}
}

func TestExtractResourceName(t *testing.T) {
	tests := []struct {
		typeName string
		expected string
	}{
		{"WidgetResource", "widget"},
		{"HttpDataSource", "http"},
		{"ComputeInstanceResource", "compute_instance"},
		{"S3BucketResource", "s3_bucket"},
		{"Widget", "widget"},
		{"DataSource", ""}, // edge case - just "DataSource" becomes empty after trimming suffix
	}

	for _, tt := range tests {
		t.Run(tt.typeName, func(t *testing.T) {
			got := matching.ExtractResourceNameExported(tt.typeName)
			if got != tt.expected {
				t.Errorf("extractResourceName(%q) = %q, want %q", tt.typeName, got, tt.expected)
			}
		})
	}
}

func TestExtractResourceNameFromPath(t *testing.T) {
	tests := []struct {
		name             string
		filePath         string
		wantResourceName string
		wantKind registry.ResourceKind
	}{
		// Prefix patterns
		{
			name:             "resource_ prefix",
			filePath:         "/path/to/resource_widget_test.go",
			wantResourceName: "widget",
			wantKind: registry.KindResource,
		},
		{
			name:             "data_source_ prefix",
			filePath:         "/path/to/data_source_http_test.go",
			wantResourceName: "http",
			wantKind: registry.KindDataSource,
		},
		{
			name:             "ephemeral_ prefix",
			filePath:         "/path/to/ephemeral_session_test.go",
			wantResourceName: "session",
			wantKind: registry.KindResource,
		},

		// Suffix patterns
		{
			name:             "_resource suffix",
			filePath:         "/path/to/widget_resource_test.go",
			wantResourceName: "widget",
			wantKind: registry.KindResource,
		},
		{
			name:             "_data_source suffix",
			filePath:         "/path/to/http_data_source_test.go",
			wantResourceName: "http",
			wantKind: registry.KindDataSource,
		},
		{
			name:             "_datasource suffix",
			filePath:         "/path/to/http_datasource_test.go",
			wantResourceName: "http",
			wantKind: registry.KindDataSource,
		},

		// Multi-part names
		{
			name:             "resource with underscores",
			filePath:         "/path/to/resource_compute_instance_test.go",
			wantResourceName: "compute_instance",
			wantKind: registry.KindResource,
		},
		{
			name:             "data source with underscores",
			filePath:         "/path/to/data_source_s3_bucket_test.go",
			wantResourceName: "s3_bucket",
			wantKind: registry.KindDataSource,
		},

		// Edge cases
		{
			name:             "not a test file",
			filePath:         "/path/to/resource_widget.go",
			wantResourceName: "",
			wantKind: registry.KindResource,
		},
		{
			name:             "no matching pattern",
			filePath:         "/path/to/helper_test.go",
			wantResourceName: "",
			wantKind: registry.KindResource,
		},
		{
			name:             "just _test.go",
			filePath:         "/path/to/_test.go",
			wantResourceName: "",
			wantKind: registry.KindResource,
		},
		{
			name:             "empty resource name after prefix",
			filePath:         "/path/to/resource__test.go",
			wantResourceName: "",
			wantKind: registry.KindResource,
		},

		// Full path variations
		{
			name:             "absolute path with prefix",
			filePath:         "/home/user/project/internal/provider/resource_bucket_test.go",
			wantResourceName: "bucket",
			wantKind: registry.KindResource,
		},
		{
			name:             "relative path with suffix",
			filePath:         "provider/http_datasource_test.go",
			wantResourceName: "http",
			wantKind: registry.KindDataSource,
		},
		{
			name:             "basename only",
			filePath:         "data_source_ami_test.go",
			wantResourceName: "ami",
			wantKind: registry.KindDataSource,
		},

		// Priority - prefix patterns should take precedence
		{
			name:             "both prefix and suffix patterns present",
			filePath:         "/path/to/resource_widget_resource_test.go",
			wantResourceName: "widget_resource",
			wantKind: registry.KindResource,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotResourceName, gotIsDataSource := matching.ExtractResourceNameFromPath(tt.filePath)
			if gotResourceName != tt.wantResourceName {
				t.Errorf("ExtractResourceNameFromPath(%q) resourceName = %q, want %q",
					tt.filePath, gotResourceName, tt.wantResourceName)
			}
			wantIsDataSource := tt.wantKind == registry.KindDataSource
			if gotIsDataSource != wantIsDataSource {
				t.Errorf("ExtractResourceNameFromPath(%q) isDataSource = %v, want %v",
					tt.filePath, gotIsDataSource, wantIsDataSource)
			}
		})
	}
}
