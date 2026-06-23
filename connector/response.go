package connector

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type Response struct {
	Location           string
	PushURL            string
	Redirect           string
	Refresh            *bool
	ReplaceURL         string
	Reswap             string
	Retarget           string
	Reselect           string
	Trigger            string
	TriggerAfterSettle string
	TriggerAfterSwap   string
}

type ResponseBuilder struct {
	response *Response
}

func NewResponseBuilder(response *Response) *ResponseBuilder {
	if response == nil {
		response = &Response{}
	}
	return &ResponseBuilder{response: response}
}

func (b *ResponseBuilder) Location(value string) *ResponseBuilder {
	b.response.Location = value
	return b
}

func (b *ResponseBuilder) PushURL(value string) *ResponseBuilder {
	b.response.PushURL = value
	return b
}

func (b *ResponseBuilder) Redirect(value string) *ResponseBuilder {
	b.response.Redirect = value
	return b
}

func (b *ResponseBuilder) Refresh(value bool) *ResponseBuilder {
	b.response.Refresh = &value
	return b
}

func (b *ResponseBuilder) ReplaceURL(value string) *ResponseBuilder {
	b.response.ReplaceURL = value
	return b
}

func (b *ResponseBuilder) Reswap(value string) *ResponseBuilder {
	b.response.Reswap = value
	return b
}

func (b *ResponseBuilder) ReswapWith(swap *Swap) *ResponseBuilder {
	if swap == nil {
		return b
	}
	b.response.Reswap = swap.String()
	return b
}

func (b *ResponseBuilder) Retarget(value string) *ResponseBuilder {
	b.response.Retarget = value
	return b
}

func (b *ResponseBuilder) Reselect(value string) *ResponseBuilder {
	b.response.Reselect = value
	return b
}

func (b *ResponseBuilder) Trigger(value string) *ResponseBuilder {
	b.response.Trigger = value
	return b
}

func (b *ResponseBuilder) TriggerWith(trigger *Trigger) *ResponseBuilder {
	if trigger == nil {
		return b
	}
	b.response.Trigger = trigger.String()
	return b
}

func (b *ResponseBuilder) TriggerAfterSettle(value string) *ResponseBuilder {
	b.response.TriggerAfterSettle = value
	return b
}

func (b *ResponseBuilder) TriggerAfterSettleWith(trigger *Trigger) *ResponseBuilder {
	if trigger == nil {
		return b
	}
	b.response.TriggerAfterSettle = trigger.String()
	return b
}

func (b *ResponseBuilder) TriggerAfterSwap(value string) *ResponseBuilder {
	b.response.TriggerAfterSwap = value
	return b
}

func (b *ResponseBuilder) TriggerAfterSwapWith(trigger *Trigger) *ResponseBuilder {
	if trigger == nil {
		return b
	}
	b.response.TriggerAfterSwap = trigger.String()
	return b
}

func (b *ResponseBuilder) Value() Response {
	return *b.response
}

func (x *base) ResponseHeaders(response Response) map[string]string {
	headers := make(map[string]string)
	setResponseHeader(headers, HeaderLocation, response.Location)
	setResponseHeader(headers, HeaderPushURL, response.PushURL)
	setResponseHeader(headers, HeaderRedirect, response.Redirect)
	if response.Refresh != nil {
		setResponseHeader(headers, HeaderRefresh, boolString(*response.Refresh))
	}
	setResponseHeader(headers, HeaderReplaceURL, response.ReplaceURL)
	setResponseHeader(headers, HeaderReswap, response.Reswap)
	setResponseHeader(headers, HeaderRetarget, response.Retarget)
	setResponseHeader(headers, HeaderReselect, response.Reselect)
	setResponseHeader(headers, HeaderTrigger, response.Trigger)
	setResponseHeader(headers, HeaderTriggerAfterSettle, response.TriggerAfterSettle)
	setResponseHeader(headers, HeaderTriggerAfterSwap, response.TriggerAfterSwap)
	return headers
}

func setResponseHeader(headers map[string]string, key HeaderKey, value string) {
	if value != "" {
		headers[key.String()] = value
	}
}

type Trigger struct {
	events map[string]any
}

func NewTrigger() *Trigger {
	return &Trigger{events: make(map[string]any)}
}

func (t *Trigger) AddEvent(event string) *Trigger {
	t.events[event] = nil
	return t
}

func (t *Trigger) AddEventDetailed(event, message string) *Trigger {
	t.events[event] = message
	return t
}

func (t *Trigger) AddEventObject(event string, details map[string]any) *Trigger {
	t.events[event] = details
	return t
}

func (t *Trigger) String() string {
	if len(t.events) == 0 {
		return ""
	}
	out, err := json.Marshal(t.events)
	if err != nil {
		return ""
	}
	return string(out)
}

type Swap struct {
	style       SwapStyle
	swap        time.Duration
	settle      time.Duration
	transition  *bool
	ignoreTitle *bool
	focusScroll *bool
	scrolling   []string
}

type SwapStyle string

const (
	SwapInnerHTML   SwapStyle = "innerHTML"
	SwapOuterHTML   SwapStyle = "outerHTML"
	SwapBeforeBegin SwapStyle = "beforebegin"
	SwapAfterBegin  SwapStyle = "afterbegin"
	SwapBeforeEnd   SwapStyle = "beforeend"
	SwapAfterEnd    SwapStyle = "afterend"
	SwapDelete      SwapStyle = "delete"
	SwapNone        SwapStyle = "none"
)

const (
	SwapModifierSwap        = "swap"
	SwapModifierSettle      = "settle"
	SwapModifierTransition  = "transition"
	SwapModifierIgnoreTitle = "ignoreTitle"
	SwapModifierFocusScroll = "focus-scroll"
	SwapModifierScroll      = "scroll"
	SwapModifierShow        = "show"
)

func NewSwap() *Swap {
	return &Swap{style: SwapInnerHTML}
}

func (s *Swap) Style(style SwapStyle) *Swap {
	s.style = style
	return s
}

func (s *Swap) Swap(duration time.Duration) *Swap {
	s.swap = duration
	return s
}

func (s *Swap) Settle(duration time.Duration) *Swap {
	s.settle = duration
	return s
}

func (s *Swap) Transition(enabled bool) *Swap {
	s.transition = &enabled
	return s
}

func (s *Swap) IgnoreTitle(enabled bool) *Swap {
	s.ignoreTitle = &enabled
	return s
}

func (s *Swap) FocusScroll(enabled bool) *Swap {
	s.focusScroll = &enabled
	return s
}

func (s *Swap) Scroll(target, direction string) *Swap {
	s.scrolling = append(s.scrolling, joinSwapOption(SwapModifierScroll, target, direction))
	return s
}

func (s *Swap) Show(target, direction string) *Swap {
	s.scrolling = append(s.scrolling, joinSwapOption(SwapModifierShow, target, direction))
	return s
}

func (s *Swap) String() string {
	if s == nil {
		return ""
	}

	parts := []string{string(s.style)}
	if s.swap > 0 {
		parts = append(parts, joinSwapOption(SwapModifierSwap, "", formatDuration(s.swap)))
	}
	if s.settle > 0 {
		parts = append(parts, joinSwapOption(SwapModifierSettle, "", formatDuration(s.settle)))
	}
	if s.transition != nil {
		parts = append(parts, joinSwapOption(SwapModifierTransition, "", boolString(*s.transition)))
	}
	if s.ignoreTitle != nil {
		parts = append(parts, joinSwapOption(SwapModifierIgnoreTitle, "", boolString(*s.ignoreTitle)))
	}
	if s.focusScroll != nil {
		parts = append(parts, joinSwapOption(SwapModifierFocusScroll, "", boolString(*s.focusScroll)))
	}
	parts = append(parts, s.scrolling...)

	return strings.Join(parts, " ")
}

func ApplyHeaders(headers http.Header, values map[string]string) {
	for key, value := range values {
		headers.Set(key, value)
	}
}

func joinSwapOption(mode, target, direction string) string {
	if target == "" {
		return mode + ":" + direction
	}
	return mode + ":" + target + ":" + direction
}

func formatDuration(duration time.Duration) string {
	if duration%time.Second == 0 {
		return duration.String()
	}
	return duration.Truncate(time.Millisecond).String()
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
