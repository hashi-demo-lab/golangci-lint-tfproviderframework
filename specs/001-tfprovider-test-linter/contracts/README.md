# API Contracts

This directory contains the API contracts for the Terraform Provider Test Coverage Linter.

## Files

### analyzer-interface.go

Defines the Go interfaces and data structures used by the linter:

- **LinterPlugin**: Main plugin interface for golangci-lint integration
- **Settings**: User configuration structure
- **ResourceRegistry**: Registry for mapping resources to tests
- **ResourceInfo, TestFileInfo**: Core data entities
- **AnalyzerFactory**: Factory for creating analyzer instances
- **ASTParser**: Utilities for AST traversal
- **DiagnosticBuilder**: Diagnostic message construction

This file is a **design specification**, not executable code. It documents the contracts that the implementation must satisfy.

### configuration-schema.yaml

Defines the configuration format for .golangci.yml and .custom-gcl.yml:

- **Settings Schema**: YAML structure for user configuration
- **Validation Rules**: Configuration constraints
- **Example Configurations**: Common usage patterns
- **Diagnostic Message Format**: Error message templates

Use this file as a reference when:
- Configuring the linter in your project
- Understanding available configuration options
- Interpreting diagnostic messages

## Usage

These contracts serve as the single source of truth for:

1. **Implementation**: Developers implement these interfaces in tfprovidertest.go
2. **Testing**: Test cases verify conformance to these contracts
3. **Documentation**: README.md references these contracts for user guidance
4. **Integration**: Other tools can understand the linter's behavior

## Contract Versioning

Contracts are versioned with the linter. Breaking changes to contracts require a major version bump.

**Current Version**: 1.0.0 (initial design)
