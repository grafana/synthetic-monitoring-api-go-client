package smapi

import "github.com/google/go-cmp/cmp"

var ignoreTimeFields = cmp.FilterPath(
	func(p cmp.Path) bool {
		switch p.String() {
		case "Created", "Modified", "OnlineChange":
			return true
		default:
			return false
		}
	},
	cmp.Ignore())

var ignoreIdField = cmp.FilterPath(
	func(p cmp.Path) bool { return p.String() == "Id" },
	cmp.Ignore())

var ignoreTenantIdField = cmp.FilterPath(
	func(p cmp.Path) bool { return p.String() == "TenantId" },
	cmp.Ignore())
