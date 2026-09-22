package xmlrecord

import "github.com/yaklang/yaklang/common/sca/core/budget"

// These fixed 64-bit layout bounds also cover 32-bit builds. POM schema tests
// check the DTO sizes and all slice/map paths against this inventory. String
// contents are charged by the token pass; these bounds cover backing arrays.
const (
	POMLicenseBytes    int64 = 16
	POMModuleBytes     int64 = 16
	POMDependencyBytes int64 = 176
	POMExclusionBytes  int64 = 32
	POMRepositoryBytes int64 = 128
)

var pomLayout = destPlan{
	slices: []destSlot{
		{path: []pathElem{{Local: "licenses"}, {Local: "license"}}, store: POMLicenseBytes},
		{path: []pathElem{{Local: "modules"}, {Local: "module"}}, store: POMModuleBytes},
		{path: []pathElem{{Local: "dependencyManagement"}, {Local: "dependencies"}, {Local: "dependency"}}, store: POMDependencyBytes},
		{path: []pathElem{{Local: "dependencyManagement"}, {Local: "dependencies"}, {Local: "dependency"}, {Local: "exclusions"}, {Local: "exclusion"}}, store: POMExclusionBytes},
		{path: []pathElem{{Local: "dependencies"}, {Local: "dependency"}}, store: POMDependencyBytes},
		{path: []pathElem{{Local: "dependencies"}, {Local: "dependency"}, {Local: "exclusions"}, {Local: "exclusion"}}, store: POMExclusionBytes},
		{path: []pathElem{{Local: "repositories"}, {Local: "repository"}}, store: POMRepositoryBytes},
	},
	properties: []destSlot{{path: []pathElem{{Local: "properties"}}, heap: budget.SizeMap + budget.SizePtr + budget.SizeObject + 2*budget.SizeString}},
}
