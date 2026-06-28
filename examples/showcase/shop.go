package main

import (
	"fmt"
	"maps"
	"net/http"
	"sort"
	"strconv"
	"strings"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
)

func (app *App) shop(w http.ResponseWriter, r *http.Request) {
	content := app.shopPartial(w, r, "content", 1, 12)
	app.renderPartial(w, r, content)
}

func (app *App) shopLoad(w http.ResponseWriter, r *http.Request) {
	action := r.Header.Get(connector.HeaderAction.String())
	if !strings.HasPrefix(action, "current-") {
		http.Error(w, "missing X-Action: current-<item>", http.StatusBadRequest)
		return
	}

	current, err := strconv.Atoi(strings.TrimPrefix(action, "current-"))
	if err != nil || current < 0 {
		http.Error(w, "invalid X-Action cursor", http.StatusBadRequest)
		return
	}

	content := app.shopPartial(w, r, "shop-chunk", current+1, 12)
	app.writeStandalone(w, r, content)
}

func (app *App) shopCartAdd(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	product := app.productByID(id)
	if err != nil || product == nil {
		http.Error(w, "unknown product", http.StatusBadRequest)
		return
	}

	sessionID := app.cartSessionID(w, r)
	app.cartMu.Lock()
	cart := app.carts[sessionID]
	if cart == nil {
		cart = make(map[int]int)
		app.carts[sessionID] = cart
	}
	cart[id]++
	app.cartMu.Unlock()

	app.writeCartUpdate(w, r, sessionID, "Added "+product.Name+" to cart")
}

func (app *App) shopCartRemove(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	product := app.productByID(id)
	if err != nil || product == nil {
		http.Error(w, "unknown product", http.StatusBadRequest)
		return
	}

	sessionID := app.cartSessionID(w, r)
	app.cartMu.Lock()
	if cart := app.carts[sessionID]; cart != nil {
		if cart[id] <= 1 {
			delete(cart, id)
		} else {
			cart[id]--
		}
	}
	app.cartMu.Unlock()

	app.writeCartUpdate(w, r, sessionID, "Removed "+product.Name)
}

func (app *App) shopCartOpen(w http.ResponseWriter, r *http.Request) {
	sessionID := app.cartSessionID(w, r)
	app.writeContent(w, r, app.shopCartPopupPartial(sessionID, true))
}

func (app *App) writeCartUpdate(w http.ResponseWriter, r *http.Request, sessionID string, message string) {
	wrapper := app.wrapper()
	wrapper.WithOOB(app.shopCartButtonPartial(sessionID))
	wrapper.WithOOB(partial.NewID("toast", "templates/toast.gohtml").SetDot(NoticePage{
		Message: message,
	}).SetAlwaysSwapOOB(true))

	content := app.shopCartPopupPartial(sessionID, true)
	layout := app.service.NewLayout().Set(content).Wrap(wrapper)
	app.writeLayout(w, r, layout)
}

func (app *App) shopPartial(w http.ResponseWriter, r *http.Request, id string, start int, count int) *partial.Partial {
	end := start + count - 1
	if end > len(app.products) {
		end = len(app.products)
	}

	items := []Product{}
	if start <= len(app.products) {
		items = append(items, app.products[start-1:end]...)
	}

	templateName := "templates/shop_chunk.gohtml"
	if id == "content" {
		templateName = "templates/shop.gohtml"
	}

	sessionID := app.cartSessionID(w, r)
	content := partial.NewID(id, templateName).SetDot(ShopPage{
		Title:        "Webshop",
		Items:        items,
		Cart:         app.cartSummary(sessionID, false),
		Start:        start,
		Next:         end,
		Done:         end >= len(app.products),
		Current:      start - 1,
		ActionHeader: connector.HeaderAction.String(),
	})
	content.With(partial.NewID("shop-item", "templates/shop_item.gohtml"))
	content.With(app.shopCartButtonPartial(sessionID))
	return content
}

func (app *App) shopCartButtonPartial(sessionID string) *partial.Partial {
	return partial.NewID("shop-cart-button", "templates/shop_cart_button.gohtml").
		SetDot(app.cartSummary(sessionID, false)).
		SetAlwaysSwapOOB(true)
}

func (app *App) shopCartPopupPartial(sessionID string, opened bool) *partial.Partial {
	return partial.NewID("cart-popup", "templates/shop_cart_popup.gohtml").
		SetDot(app.cartSummary(sessionID, opened)).
		SetAlwaysSwapOOB(true)
}

func (app *App) cartSummary(sessionID string, opened bool) CartSummary {
	app.cartMu.Lock()
	cart := maps.Clone(app.carts[sessionID])
	app.cartMu.Unlock()

	ids := make([]int, 0, len(cart))
	for id := range cart {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	summary := CartSummary{Opened: opened}
	for _, id := range ids {
		product := app.productByID(id)
		if product == nil {
			continue
		}
		quantity := cart[id]
		lineCents := product.PriceCents * quantity
		summary.Count += quantity
		summary.TotalCents += lineCents
		summary.Lines = append(summary.Lines, CartLine{
			Product:   *product,
			Quantity:  quantity,
			LineCents: lineCents,
			LineTotal: formatCents(lineCents),
		})
	}
	summary.Total = formatCents(summary.TotalCents)
	summary.Empty = summary.Count == 0
	return summary
}

func (app *App) productByID(id int) *Product {
	for i := range app.products {
		if app.products[i].ID == id {
			return &app.products[i]
		}
	}
	return nil
}

func (app *App) cartSessionID(w http.ResponseWriter, r *http.Request) string {
	const cookieName = "go_partial_showcase_cart"
	cookie, err := r.Cookie(cookieName)
	if err != nil || cookie.Value == "" {
		cookie = &http.Cookie{
			Name:     cookieName,
			Value:    randomID(),
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		}
		http.SetCookie(w, cookie)
	}
	return cookie.Value
}

func fakeProducts() []Product {
	names := []string{
		"Canvas Tote", "Desk Lamp", "Notebook Set", "Ceramic Mug", "Wool Scarf",
		"Brass Key Hook", "Travel Pouch", "Linen Apron", "Oak Tray", "Glass Carafe",
		"Pocket Planner", "Cotton Throw", "Market Basket", "Steel Bottle", "Reading Light",
		"Soap Trio", "Herb Seeds", "Bamboo Stand", "Cork Coasters", "Enamel Bowl",
		"Sketch Pencils", "Storage Crate", "Tea Towels", "Plant Mister", "Wall Calendar",
		"Felt Desk Mat", "Lunch Box", "Incense Holder", "Mini Speaker", "Cable Wrap",
		"Recipe Cards", "Garden Gloves", "Photo Clips", "Serving Spoon", "Woven Mat",
		"Matcha Whisk", "Sleep Mask", "Stone Vase", "Bike Bell", "Poster Frame",
		"Bath Salts", "Pantry Labels", "Pepper Mill", "Window Planter", "Picnic Blanket",
		"Coffee Scoop", "Shoe Brush", "Table Runner", "Candle Snuffer", "Fruit Bowl",
	}
	categories := []string{"Home", "Desk", "Kitchen", "Garden", "Travel"}
	accents := []string{"#e0f0eb", "#f4e6c8", "#e9e4f5", "#f7dfd2", "#dfe8f6"}

	products := make([]Product, 0, len(names))
	for i, name := range names {
		price := 795 + ((i*137 + 419) % 4200)
		products = append(products, Product{
			ID:          i + 1,
			Name:        name,
			Category:    categories[i%len(categories)],
			PriceCents:  price,
			Price:       formatCents(price),
			Description: fmt.Sprintf("Small-batch %s for calm daily routines.", strings.ToLower(name)),
			Accent:      accents[i%len(accents)],
		})
	}
	return products
}

func formatCents(cents int) string {
	return fmt.Sprintf("EUR %d.%02d", cents/100, cents%100)
}
