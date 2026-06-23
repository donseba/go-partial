package main

func (app *App) navItems() []NavItem {
	return []NavItem{
		{Path: "/", Label: "Home", Group: "Start"},
		{Path: "/scoped", Label: "Scoped rows", Group: "Core rendering"},
		{Path: "/context", Label: "Context", Group: "Core rendering"},
		{Path: "/localization", Label: "Localization", Group: "Core rendering"},
		{Path: "/selection", Label: "Selection", Group: "Interactions"},
		{Path: "/tabs", Label: "Tabs", Group: "Interactions"},
		{Path: "/action", Label: "Actions", Group: "Interactions"},
		{Path: "/flow", Label: "Flow", Group: "Interactions"},
		{Path: "/infinite", Label: "Infinite scroll", Group: "Interactions"},
		{Path: "/shop", Label: "Webshop", Group: "Interactions"},
		{Path: "/interactions", Label: "Interaction helpers", Group: "Client interactions"},
		{Path: "/async", Label: "Async rows", Group: "Client interactions"},
		{Path: "/oob", Label: "OOB", Group: "Integrations"},
		{Path: "/headers", Label: "Headers", Group: "Integrations"},
		{Path: "/sse", Label: "SSE", Group: "Integrations"},
		{Path: "/debug", Label: "Debug", Group: "Diagnostics"},
		{Path: "/error", Label: "Error", Group: "Diagnostics"},
	}
}
