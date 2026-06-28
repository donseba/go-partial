package main

import (
	"html/template"
	"sync"

	partial "github.com/donseba/go-partial"
)

type Row struct {
	ID     int
	Name   string
	Price  string
	Status string
	Owner  string
}

type RowsPage struct {
	Title string
	Rows  []Row
}

type PageTitle struct {
	Title string
}

type ActionPage struct {
	Title        string
	Counter      int
	ActionHeader string
}

type AsyncPage struct {
	Title string
	Rows  []Row
}

type AsyncStats struct {
	RenderedAt string
	Rows       int
}

type AsyncRow struct {
	Row        Row
	RenderedAt string
}

type DebugPage struct {
	Title       string
	Payload     map[string]any
	CustomDebug template.HTML
}

type DebugCustomPage struct {
	Name string
	Role string
}

type FlowPage struct {
	Title       string
	Steps       []partial.FlowStep
	CurrentStep string
	Validated   map[string]bool
	Error       string
	Account     FlowAccountPage
	Details     FlowDetailsPage
	Confirm     FlowConfirmPage
}

type FlowAccountPage struct {
	Email string
	Error string
}

type FlowDetailsPage struct {
	Name  string
	Plan  string
	Error string
}

type FlowConfirmPage struct {
	AllData map[string]any
}

type InfinitePage struct {
	Title        string
	Rows         []InfiniteRow
	Next         int
	Done         bool
	Start        int
	Current      int
	ActionHeader string
}

type InfiniteRow struct {
	Number int
}

type InfiniteToast struct {
	Start        int
	Next         int
	Current      int
	ActionHeader string
}

type InteractionPage struct {
	Title    string
	Interact InteractionSet
}

type InteractionSet struct {
	Async          partial.Interaction
	Poll           partial.Interaction
	On             partial.Interaction
	Refresh        partial.Interaction
	Profile        partial.Interaction
	ProfileRefresh partial.Interaction
	Stream         partial.Interaction
	Prefetch       partial.Interaction
	Reveal         partial.Interaction
}

type InteractionResult struct {
	ID      string
	Label   string
	Message string
	Time    string
}

type LocalizationPage struct {
	Title   string
	Locale  string
	Locales []string
	Count   int
	Loc     partial.Localizer
}

type NoticePage struct {
	Message string
}

type SSEStatus struct {
	Step int
	Time string
	Done bool
}

type TabItem struct {
	Key   string
	Label string
}

type TabsPage struct {
	Title string
	Tabs  []TabItem
}

type SelectionPanel struct {
	Title string
}

type Product struct {
	ID          int
	Name        string
	Category    string
	PriceCents  int
	Price       string
	Description string
	Accent      string
}

type ShopPage struct {
	Title        string
	Items        []Product
	Cart         CartSummary
	Start        int
	Next         int
	Done         bool
	Current      int
	ActionHeader string
}

type CartLine struct {
	Product   Product
	Quantity  int
	LineCents int
	LineTotal string
}

type CartSummary struct {
	Lines      []CartLine
	Count      int
	TotalCents int
	Total      string
	Empty      bool
	Opened     bool
}

type NavItem struct {
	Path  string
	Label string
	Group string
}

type HeaderPage struct {
	AppName string
	Now     string
	Nav     []NavItem
	Joke    string
}

type ShellPage struct {
	AppName string
	Header  HeaderPage
}

type App struct {
	service      *partial.Service
	rows         []Row
	products     []Product
	carts        map[string]map[int]int
	cartMu       sync.Mutex
	counter      int
	flowSessions map[string]*partial.FlowSessionData
}
