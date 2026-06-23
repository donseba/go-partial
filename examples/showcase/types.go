package main

import (
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

type Product struct {
	ID          int
	Name        string
	Category    string
	PriceCents  int
	Price       string
	Description string
	Accent      string
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

type App struct {
	service      *partial.Service
	rows         []Row
	products     []Product
	carts        map[string]map[int]int
	cartMu       sync.Mutex
	counter      int
	flowSessions map[string]*partial.FlowSessionData
}
