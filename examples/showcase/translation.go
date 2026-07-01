package main

import (
	"context"
	"fmt"
	"html/template"

	"github.com/donseba/go-partial/exp/localization"
)

type showcaseLocalizer struct {
	locale string
}

type showcaseCsrf struct {
	key   string
	token string
}

func (c showcaseCsrf) Token(ctx context.Context) string {
	return c.token
}

func (c showcaseCsrf) Key() string {
	return c.key
}

func (l showcaseLocalizer) GetLocale() string {
	return l.locale
}

type showcaseTranslator struct {
	messages map[string]map[string]string
}

func (t showcaseTranslator) FuncMap() template.FuncMap {
	return template.FuncMap{
		"tl":  t.tl,
		"tn":  t.tn,
		"ctl": t.ctl,
		"ctn": t.ctn,
	}
}

func (t showcaseTranslator) tl(loc localization.Localizer, key string, args ...any) string {
	return t.translate(loc.GetLocale(), key, args...)
}

func (t showcaseTranslator) tn(loc localization.Localizer, singular string, plural string, n int, args ...any) string {
	key := plural
	if n == 1 {
		key = singular
	}
	return t.translate(loc.GetLocale(), key, args...)
}

func (t showcaseTranslator) ctl(loc localization.Localizer, context string, key string, args ...any) string {
	return t.translate(loc.GetLocale(), context+"."+key, args...)
}

func (t showcaseTranslator) ctn(loc localization.Localizer, context string, singular string, plural string, n int, args ...any) string {
	key := plural
	if n == 1 {
		key = singular
	}
	return t.translate(loc.GetLocale(), context+"."+key, args...)
}

func (t showcaseTranslator) translate(locale string, key string, args ...any) string {
	values, ok := t.messages[locale]
	if !ok {
		values = t.messages["en_US"]
	}
	value, ok := values[key]
	if !ok {
		value = key
	}
	if len(args) > 0 {
		return fmt.Sprintf(value, args...)
	}
	return value
}

func showcaseTranslationFunctions() template.FuncMap {
	return showcaseTranslator{messages: map[string]map[string]string{
		"en_US": {
			"title":       "Localization",
			"intro":       "The localizer is stored in the request context and exposed to every template as localizer.",
			"checkout":    "Checkout",
			"oneMessage":  "You have one message.",
			"manyMessage": "You have %d messages.",
			"total":       "Total",
			"delivery":    "Delivery",
			"status":      "Ready for pickup",
			"active":      "Active locale",
			"button.save": "Save changes",
			"explanation": "Switch languages with HTMX and the content re-renders from the server without replacing the page shell.",
		},
		"nl_NL": {
			"title":       "Lokalisatie",
			"intro":       "De localizer staat in de request-context en is in elke template beschikbaar als localizer.",
			"checkout":    "Afrekenen",
			"oneMessage":  "Je hebt een bericht.",
			"manyMessage": "Je hebt %d berichten.",
			"total":       "Totaal",
			"delivery":    "Bezorging",
			"status":      "Klaar om op te halen",
			"active":      "Actieve taal",
			"button.save": "Wijzigingen opslaan",
			"explanation": "Wissel van taal met HTMX en de server rendert de inhoud opnieuw zonder de pagina-shell te vervangen.",
		},
		"fr_FR": {
			"title":       "Localisation",
			"intro":       "Le localizer vit dans le contexte de la requete et chaque template le lit avec localizer.",
			"checkout":    "Paiement",
			"oneMessage":  "Vous avez un message.",
			"manyMessage": "Vous avez %d messages.",
			"total":       "Total",
			"delivery":    "Livraison",
			"status":      "Pret pour le retrait",
			"active":      "Langue active",
			"button.save": "Enregistrer",
			"explanation": "Changez de langue avec HTMX et le serveur rend le contenu sans remplacer la structure de la page.",
		},
	}}.FuncMap()
}
