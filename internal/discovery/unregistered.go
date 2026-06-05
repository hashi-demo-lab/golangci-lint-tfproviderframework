package discovery

import (
	"go/ast"
	"go/token"

	"github.com/example/tfprovidertest/internal/registry"
)

// resourceKey identifies a resource by canonical name and kind.
type resourceKey struct {
	name string
	kind registry.ResourceKind
}

// DetectUnregisteredResources flags resources that are defined in source — they
// have a framework constructor (NewXxx returning resource.Resource /
// datasource.DataSource / action.Action) and a Schema/Metadata — but whose
// constructor is not listed in the provider's Resources()/DataSources()/
// Actions() aggregator methods. Such a resource exists in code but never ships
// (finding F3, e.g. terraform-provider-powerhmc's lpar data source).
//
// The check only applies to providers that use the aggregator pattern: if no
// Resources()/DataSources()/Actions() method is found, it is a no-op (SDKv2 and
// registry-factory providers register differently and are never flagged).
//
// Constructor -> canonical name resolution goes through the returned type's
// Metadata (NewSysCfgDataSource -> sysCfgDataSource -> "sys_config"), so a
// Metadata-renamed but registered resource is not falsely flagged.
func DetectUnregisteredResources(files []*ast.File, fset *token.FileSet, reg *registry.ResourceRegistry) {
	registeredCtors, hasAggregator, complete := collectAggregatorConstructors(files)
	if !hasAggregator || !complete {
		// No aggregator pattern, or an aggregator we cannot fully read (e.g. it
		// spreads a builder slice from another package, as terraform-provider-hcp
		// does with `packer.ResourceSchemaBuilders...`). In the incomplete case we
		// cannot know the true registered set, so flag nothing rather than risk
		// false positives.
		return
	}

	// Map every framework constructor to the (canonical name, kind) it produces.
	ctorToResource := collectConstructorResources(files)
	if len(ctorToResource) == 0 {
		return
	}

	registered := make(map[resourceKey]bool)
	for ctor := range registeredCtors {
		if rk, ok := ctorToResource[ctor]; ok {
			registered[rk] = true
		}
	}

	hasConstructor := make(map[resourceKey]bool, len(ctorToResource))
	for _, rk := range ctorToResource {
		hasConstructor[rk] = true
	}

	for _, info := range reg.GetAllDefinitions() {
		key := resourceKey{name: info.Name, kind: info.Kind}
		// Only consider resources that actually have a framework constructor;
		// flag those whose constructor is absent from the aggregator.
		if hasConstructor[key] && !registered[key] {
			info.Unregistered = true
		}
	}
}

// collectAggregatorConstructors scans every file for provider aggregator methods
// (Resources/DataSources/Actions) and returns the set of constructor identifiers
// listed in their returned slices, whether any such method was found, and
// whether the aggregator could be read completely. "Complete" is false when a
// method spreads a value we cannot resolve to a literal constructor list (e.g.
// `append(base, somepkg.Builders...)`), in which case the registered set is
// unknown and callers should not flag anything.
func collectAggregatorConstructors(files []*ast.File) (ctors map[string]bool, found, complete bool) {
	ctors = make(map[string]bool)
	complete = true

	for _, file := range files {
		for _, decl := range file.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Recv == nil {
				continue
			}
			switch funcDecl.Name.Name {
			case "Resources", "DataSources", "Actions":
			default:
				continue
			}
			found = true
			ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
				switch e := n.(type) {
				case *ast.Ident:
					// Identifier appearing in the body (constructor names in the
					// []func() X{ NewA, NewB } slice). Non-constructor idents are
					// harmless — they match nothing in ctorToResource.
					ctors[e.Name] = true
				case *ast.CallExpr:
					// A spread call such as append(base, pkg.Builders...) hides
					// constructors we cannot read; the registered set is then
					// incomplete.
					if e.Ellipsis != token.NoPos {
						complete = false
					}
				}
				return true
			})
		}
	}
	return ctors, found, complete
}

// collectConstructorResources maps each framework constructor function name to
// the (canonical name, kind) of the resource it produces, resolving the name via
// the returned type's Metadata method.
func collectConstructorResources(files []*ast.File) map[string]resourceKey {
	// typeName -> canonical resource name (from each type's Metadata method).
	typeToName := make(map[string]string)
	for _, file := range files {
		for _, decl := range file.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Recv == nil || funcDecl.Name.Name != "Metadata" {
				continue
			}
			recvType := getReceiverTypeName(funcDecl.Recv)
			if recvType == "" {
				continue
			}
			if name := extractTypeNameFromMetadata(funcDecl.Body); name != "" {
				typeToName[recvType] = name
			}
		}
	}

	out := make(map[string]resourceKey)
	for _, file := range files {
		aliases := extractImportAliases(file)
		for _, decl := range file.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Recv != nil || funcDecl.Type.Results == nil {
				continue
			}
			if len(funcDecl.Type.Results.List) != 1 {
				continue
			}
			kind, isResource := classifyReturnType(typeToString(funcDecl.Type.Results.List[0].Type), aliases)
			if !isResource {
				continue
			}
			returnedType := extractReturnedTypeName(funcDecl.Body)
			if returnedType == "" {
				continue
			}
			name, ok := typeToName[returnedType]
			if !ok {
				continue
			}
			out[funcDecl.Name.Name] = resourceKey{name: name, kind: kind}
		}
	}
	return out
}
