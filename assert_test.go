package smapi

import "github.com/google/go-cmp/cmp"

func ignoreTimeFields() cmp.Option {
	return cmp.FilterPath(
		func(p cmp.Path) bool {
			switch p.String() {
			case "Created", "Modified", "OnlineChange":
				return true
			default:
				return false
			}
		},
		cmp.Ignore())
}

func ignoreIDField() cmp.Option {
	return cmp.FilterPath(
		func(p cmp.Path) bool { return p.String() == "Id" },
		cmp.Ignore())
}

func ignoreTenantIDField() cmp.Option {
	return cmp.FilterPath(
		func(p cmp.Path) bool { return p.String() == "TenantId" },
		cmp.Ignore())
}
