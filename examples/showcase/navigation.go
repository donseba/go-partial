package main

func (app *App) navItems() []NavItem {
	return []NavItem{
		{Path: "/", Label: "Home", Group: "Start"},
		{Path: "/rows", Label: "Typed rows", Group: "Core rendering"},
		{Path: "/context", Label: "Context", Group: "Core rendering"},
		{Path: "/localization", Label: "Localization", Group: "exp"},
		{Path: "/selection", Label: "Selection", Group: "exp"},
		{Path: "/tabs", Label: "Tabs", Group: "exp"},
		{Path: "/action", Label: "Actions", Group: "exp"},
		{Path: "/flow", Label: "Flow", Group: "exp"},
		{Path: "/infinite", Label: "Infinite scroll", Group: "exp"},
		{Path: "/shop", Label: "Webshop", Group: "exp"},
		{Path: "/interactions", Label: "Interaction helpers", Group: "exp"},
		{Path: "/async", Label: "Async rows", Group: "exp"},
		{Path: "/sse", Label: "SSE", Group: "exp"},
		{Path: "/metrics", Label: "Metrics", Group: "exp"},
		{Path: "/metrics/live", Label: "Live metrics", Group: "exp"},
		{Path: "/debug", Label: "Debug", Group: "ext"},
		{Path: "/error", Label: "Error", Group: "ext"},
		{Path: "/logger", Label: "Logger", Group: "ext"},
		{Path: "/oob", Label: "OOB", Group: "Integrations"},
		{Path: "/headers", Label: "Headers", Group: "Integrations"},
	}
}
