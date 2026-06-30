package main

import (
	"html/template"
	"sync"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/exp/pageflow"
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
	Steps       []pageflow.Step
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
	Title string
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
}

type NoticePage struct {
	Message string
}

type SSEStatus struct {
	Step int
	Time string
	Done bool
}

type MetricsPage struct {
	Title      string
	Total      int
	Latest     []MetricsTraceView
	ChainTag   string
	TraceLabel string
}

type MetricsTraceView struct {
	RequestID string
	TraceID   string
	Timestamp string
	Method    string
	Path      string
	Records   []MetricsRecordView
}

type MetricsRecordView struct {
	Kind            string
	Name            string
	RequestID       string
	TraceID         string
	ParentRequestID string
	Timestamp       string
	Label           string
	PartialID       string
	ParentID        string
	Depth           int
	Indent          string
	Meta            []string
	PartialLabel    string
	SlotName        string
	Templates       string
	Swap            string
	Method          string
	Path            string
	Size            string
	Duration        string
	Error           string
	Chain           string
}

type LiveMetricsPage struct {
	Title string
}

type LiveMetricRow struct {
	Timestamp string
	Kind      string
	Label     string
	Meta      []string
	Templates string
	Swap      string
	Method    string
	Path      string
	Duration  string
	Size      string
	Chain     string
	Error     string
}

type LiveMetricPing struct {
	Time string
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
	service       *partial.Service
	events        *partial.AsyncEvents
	rows          []Row
	products      []Product
	carts         map[string]map[int]int
	cartMu        sync.Mutex
	counter       int
	flowSessions  map[string]*pageflow.SessionData
	metrics       *showcaseMetrics
	metricStreams *metricStreamHub
	logs          *showcaseLogs
}

type LoggerPage struct {
	Title       string
	Total       int
	Visible     int
	Latest      []LogRecordView
	LevelFilter string
	DebugURL    string
	InfoURL     string
}

type LogRecordView struct {
	Timestamp string
	Level     string
	Kind      string
	Message   string
	PartialID string
	Request   string
	TraceID   string
	Fields    []string
	Error     string
}
