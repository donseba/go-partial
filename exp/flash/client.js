// go-partial flash client: hides the flash messages of exp/flash after a
// while, keeps them while the pointer or keyboard focus is on them, removes
// them when a [data-flash-dismiss] element in them is clicked or Escape is
// pressed in them, and can show a message for requests that fail.
//
// Messages are elements with data-flash-level, as exp/flash's templates
// write them. Each level hides after a default time (success and info 5s,
// warn 8s, error never), which an ancestor changes with
// data-flash-autohide-<level>="3s" or, for every level,
// data-flash-autohide="3s", and a message with its own
// data-flash-autohide; "off" keeps it. Every message dispatches
// partial:flash on itself as it appears, with its level and text.
//
// Messages the client shows itself, with goPartialFlash.show(level, text)
// or for a failed request, are clones of a server-rendered
// <template data-flash-template="error"> (or data-flash-template without a
// level, for every level), such as exp/flash's {{ flashTemplate }} writes
// with the application's flash template: the text goes into its
// [data-flash-text] elements, or replaces exp/flash's text marker, and the
// clone is added where the template is. Without a template, the client
// writes exp/flash's default markup in the flash target.
//
// With data-flash-request-error="Something went wrong" on the flash target
// (exp/flash's {{ flashTarget }}), requests of htmx (2 and 4), Unpoly or
// Turbo that fail, and partial:error events of exp/sse, show that text as
// an error. A response with an error status that htmx swaps into its
// target, such as a form with its validation errors (422) with htmx 4, is
// shown by that swap and does not count as failed. Attributes on the flash
// target, which exp/flash's {{ flashTargetAttrs }} writes, change how:
//
//	data-flash-request-error-replace     one request error at a time: a new
//	                                     one replaces the last, and the
//	                                     next request removes it
//	data-flash-request-error-body        a refused request (4xx) with a
//	                                     text/plain sentence as its body
//	                                     shows that sentence; htmx does not
//	                                     swap that body
//	data-flash-request-error-skip-flash  a failed response that brings its
//	                                     own [data-flash-level] message,
//	                                     such as ext/errors' WithFlash or
//	                                     flash.Respond, shows only that one
//
// Scripts call goPartialFlash.show(level, text) and
// goPartialFlash.dismiss(element).
(function (global) {
    "use strict";

    if (!global.document || global.goPartialFlash) return;

    var document = global.document;
    var defaults = { success: 5000, info: 5000, warn: 8000, error: 0 };
    var messages = new WeakMap();
    var selector = "[data-flash-level]";
    // textMarker stands for the text in exp/flash's {{ flashTemplate }}.
    var textMarker = "__partial_flash_text__";
    var requestErrorAttr = "data-flash-request-error-message";
    // failing is set while the response of a failed request is swapped:
    // the error messages it brings count as request errors.
    var failing = 0;
    var pending = [];

    // duration reads "5s", "300ms", "2m" or a number of milliseconds; "off"
    // is 0, which keeps the message.
    function duration(value) {
        if (value === null || value === undefined) return null;
        value = String(value).trim();
        if (value === "off" || value === "never" || value === "false") return 0;
        var match = /^(\d+(?:\.\d+)?)\s*(ms|s|m)?$/.exec(value);
        if (!match) return null;
        var amount = parseFloat(match[1]);
        if (match[2] === "s") return amount * 1000;
        if (match[2] === "m") return amount * 60000;
        return amount;
    }

    function autohide(element, level) {
        var own = duration(element.getAttribute("data-flash-autohide"));
        if (own !== null) return own;
        for (var parent = element.parentElement; parent; parent = parent.parentElement) {
            var byLevel = duration(parent.getAttribute("data-flash-autohide-" + level));
            if (byLevel !== null) return byLevel;
            var all = duration(parent.getAttribute("data-flash-autohide"));
            if (all !== null) return all;
        }
        return Object.prototype.hasOwnProperty.call(defaults, level) ? defaults[level] : defaults.info;
    }

    function emit(target, name, detail) {
        target.dispatchEvent(new CustomEvent(name, { bubbles: true, detail: detail }));
    }

    function dismiss(element) {
        if (!element || !element.isConnected) return;
        var state = messages.get(element);
        if (state && state.timer) clearTimeout(state.timer);
        messages.delete(element);

        var group = element.parentElement && element.parentElement.closest(".partial-flash");
        element.remove();
        if (group && !group.querySelector(selector)) group.remove();
    }

    // schedule hides the message when its time is up; a message without a
    // time (autohide 0) stays.
    function schedule(element, state) {
        if (!state.autohide || state.paused) return;
        state.started = Date.now();
        state.timer = setTimeout(function () {
            dismiss(element);
        }, Math.max(state.remaining, 0));
    }

    function pause(element) {
        var state = messages.get(element);
        if (!state || state.paused) return;
        state.paused = true;
        if (state.timer) {
            clearTimeout(state.timer);
            state.timer = null;
            state.remaining -= Date.now() - state.started;
        }
    }

    function resume(element) {
        var state = messages.get(element);
        if (!state || !state.paused) return;
        if (element.matches(":hover") || element.contains(document.activeElement)) return;
        state.paused = false;
        schedule(element, state);
    }

    function messageText(element) {
        var text = element.querySelector("[data-flash-text]") || element.querySelector(".partial-flash__text");
        return (text || element).textContent.trim();
    }

    function track(element) {
        if (messages.has(element)) return;
        var level = element.getAttribute("data-flash-level") || "info";
        if (failing > 0 && level === "error" && !element.hasAttribute(requestErrorAttr)) {
            element.setAttribute(requestErrorAttr, "");
            if (options().replace) dismissRequestErrors(element);
        }

        var hide = autohide(element, level);
        var state = { autohide: hide, remaining: hide, paused: false, timer: null, started: 0 };
        messages.set(element, state);

        element.addEventListener("mouseenter", function () { pause(element); });
        element.addEventListener("focusin", function () { pause(element); });
        element.addEventListener("mouseleave", function () { resume(element); });
        element.addEventListener("focusout", function () { setTimeout(function () { resume(element); }, 0); });

        emit(element, "partial:flash", {
            level: level,
            text: messageText(element),
            autohide: hide
        });
        schedule(element, state);
    }

    function scan(root) {
        if (root.matches && root.matches(selector)) track(root);
        if (root.querySelectorAll) Array.prototype.forEach.call(root.querySelectorAll(selector), track);
    }

    function target() {
        return document.querySelector("[data-flash-request-error]") ||
            document.querySelector(".partial-flash-target") ||
            document.getElementById("flash-messages");
    }

    function options() {
        var container = target();
        if (!container) return { text: "", replace: false, body: false, skipFlash: false };
        return {
            text: container.getAttribute("data-flash-request-error") || "",
            replace: container.hasAttribute("data-flash-request-error-replace"),
            body: container.hasAttribute("data-flash-request-error-body"),
            skipFlash: container.hasAttribute("data-flash-request-error-skip-flash")
        };
    }

    // template returns the <template data-flash-template> for level: one
    // of the level, or else one without, in the flash target first.
    function template(level) {
        var scopes = [target(), document].filter(Boolean);
        for (var i = 0; i < scopes.length; i++) {
            var templates = scopes[i].querySelectorAll("template[data-flash-template]");
            var general = null;
            for (var j = 0; j < templates.length; j++) {
                var value = templates[j].getAttribute("data-flash-template");
                if (value === level) return templates[j];
                if (!value && !general) general = templates[j];
            }
            if (general) return general;
        }
        return null;
    }

    // fill puts text in a clone of a template: in its [data-flash-text]
    // elements, or in place of the text marker in its text and attributes.
    function fill(element, text) {
        var slots = element.matches("[data-flash-text]") ? [element] : [];
        slots = slots.concat(Array.prototype.slice.call(element.querySelectorAll("[data-flash-text]")));
        if (slots.length) {
            slots.forEach(function (slot) { slot.textContent = text; });
            return;
        }

        var walker = document.createTreeWalker(element, NodeFilter.SHOW_ELEMENT | NodeFilter.SHOW_TEXT);
        var found = false;
        for (var node = walker.currentNode; node; node = walker.nextNode()) {
            if (node.nodeType === 3) {
                if (node.data.indexOf(textMarker) >= 0) {
                    node.data = node.data.split(textMarker).join(text);
                    found = true;
                }
                continue;
            }
            Array.prototype.forEach.call(node.attributes, function (attr) {
                if (attr.value.indexOf(textMarker) >= 0) node.setAttribute(attr.name, attr.value.split(textMarker).join(text));
            });
        }
        if (!found) element.appendChild(document.createTextNode(text));
    }

    function fromTemplate(source, level, text) {
        var first = source.content.firstElementChild;
        if (!first) return null;
        var message = first.cloneNode(true);
        if (!message.matches(selector) && !message.querySelector(selector)) {
            message.setAttribute("data-flash-level", level);
        }
        fill(message, text);
        return message;
    }

    // builtIn writes a message as exp/flash's default template does.
    function builtIn(container, level, text) {
        var group = container.querySelector(".partial-flash");
        if (!group) {
            group = document.createElement("div");
            group.className = "partial-flash";
            group.setAttribute("role", "status");
            group.setAttribute("aria-live", "polite");
            container.appendChild(group);
        }

        var message = document.createElement("div");
        message.className = "partial-flash__message partial-flash__message--" + level;
        message.setAttribute("data-flash-level", level);
        var strong = document.createElement("strong");
        strong.className = "partial-flash__level";
        strong.textContent = level;
        var span = document.createElement("span");
        span.className = "partial-flash__text";
        span.textContent = text;
        message.appendChild(strong);
        message.appendChild(span);
        group.appendChild(message);
        return message;
    }

    // show adds a message: a clone of the flash template of its level, or
    // exp/flash's default markup in the flash target.
    function show(level, text, requestError) {
        level = level || "info";
        if (!text) return null;

        var message = null;
        var source = template(level);
        if (source && source.parentNode) {
            message = fromTemplate(source, level, text);
            if (message) {
                var inner = message.matches(selector) ? message : message.querySelector(selector);
                if (requestError && inner) inner.setAttribute(requestErrorAttr, "");
                source.parentNode.appendChild(message);
            }
        }
        if (!message) {
            var container = target();
            if (!container) return null;
            message = builtIn(container, level, text);
            if (requestError) message.setAttribute(requestErrorAttr, "");
        }

        var element = message.matches(selector) ? message : message.querySelector(selector);
        if (element) track(element);
        return element || message;
    }

    function dismissRequestErrors(except) {
        Array.prototype.forEach.call(document.querySelectorAll("[" + requestErrorAttr + "]"), function (element) {
            if (element !== except) dismiss(element);
        });
    }

    // readable is the body of a refused request when it is a sentence for
    // people, such as "You may not do this."; machine codes such as
    // invalid_form, markup and Go's "404 page not found" are not.
    function readable(text) {
        var message = (text || "").trim();
        if (!message || message.length > 300 || /^[a-z0-9_.-]+$/.test(message) || /^\d{3} /.test(message) || /</.test(message)) return "";
        return message;
    }

    // requestFailed shows the error of a failed request: its own message
    // when the body option allows, else the flash target's text.
    function requestFailed(response) {
        var config = options();
        response = response || {};
        var plain = /^text\/plain/i.test(response.contentType || "");

        if (config.skipFlash && !plain && response.text && response.text.indexOf("data-flash-level") >= 0) return;

        var text = "";
        if (config.body && plain && response.status >= 400 && response.status < 500) text = readable(response.text);
        text = text || config.text;
        if (!text) return;

        if (config.replace) dismissRequestErrors();
        show("error", text, true);
    }

    // failingResponse counts a failed response whose swap is under way,
    // until endFailing, or five seconds at most.
    function failingResponse() {
        failing++;
        var ended = false;
        function end() {
            if (ended) return;
            ended = true;
            failing = Math.max(failing - 1, 0);
        }
        setTimeout(end, 5000);
        pending.push(end);
    }

    function endFailing() {
        var ends = pending;
        pending = [];
        // The swap is done; the messages it added are tracked first.
        setTimeout(function () { ends.forEach(function (end) { end(); }); }, 0);
    }

    function plainText(contentType) {
        return /^text\/plain/i.test(contentType || "");
    }

    // htmx 4 dispatches htmx:response:error for every response with an
    // error status, before it decides whether to swap it: a page with an
    // error status, such as a form with its validation errors (422), is
    // swapped like any other unless hx-status or the config's noSwap says
    // otherwise. So the response is kept, and the request counts as failed
    // at htmx:finally:request only when nothing of it was swapped into its
    // target. A text/plain body shown as the message is not swapped.
    var failedResponses = new WeakMap();

    document.addEventListener("htmx:response:error", function (event) {
        var ctx = event.detail && event.detail.ctx;
        if (!ctx || !ctx.response) return;
        var headers = ctx.response.headers;
        var contentType = headers && headers.get ? headers.get("Content-Type") : "";
        if (options().body && plainText(contentType)) {
            ctx.swap = "none";
            ctx.push = false;
        }
        failingResponse();
        failedResponses.set(ctx, { status: ctx.response.status, text: ctx.text, contentType: contentType });
    });

    // swapped reports whether htmx 4 swapped the response of ctx into its
    // target: its status says so, its swap is not "none" and it had a body.
    // Out-of-band parts, such as a flash message, do not count.
    function swapped(ctx) {
        if (ctx.status !== "swapped") return false;
        var style = String(ctx.swap || "").trim().split(/\s+/)[0];
        if (style === "none") return false;
        return !!(ctx.text && ctx.text.trim());
    }

    // navigated reports whether the response sent the browser elsewhere,
    // with HX-Redirect, HX-Location or HX-Refresh.
    function navigated(ctx) {
        var hx = ctx.hx || {};
        return !!(hx.redirect || hx.location || hx.refresh === "true");
    }

    document.addEventListener("htmx:finally:request", function (event) {
        var ctx = event.detail && event.detail.ctx;
        var response = ctx && failedResponses.get(ctx);
        if (response) {
            failedResponses.delete(ctx);
            if (!swapped(ctx) && !navigated(ctx)) requestFailed(response);
        }
        endFailing();
    });

    // htmx 2: a response with an error status, which htmx 2 does not swap
    // unless its responseHandling config says so; one it swaps does not
    // count as failed. htmx:beforeSwap says whether it does, before
    // htmx:responseError. A text/plain body shown as the message is not
    // swapped.
    var swapDecisions = new WeakMap();

    document.addEventListener("htmx:beforeSwap", function (event) {
        var detail = event.detail;
        if (!detail || !detail.xhr || detail.xhr.status < 400) return;
        if (options().body && plainText(detail.xhr.getResponseHeader("Content-Type"))) detail.shouldSwap = false;
        swapDecisions.set(detail.xhr, detail);
    });

    document.addEventListener("htmx:responseError", function (event) {
        var xhr = event.detail && event.detail.xhr;
        if (!xhr) return requestFailed();
        var decision = swapDecisions.get(xhr);
        swapDecisions.delete(xhr);
        if (decision && decision.shouldSwap && (xhr.responseText || "").trim()) return;
        failingResponse();
        requestFailed({ status: xhr.status, text: xhr.responseText, contentType: xhr.getResponseHeader("Content-Type") });
    });
    document.addEventListener("htmx:afterRequest", endFailing);
    document.addEventListener("htmx:sendError", function () { requestFailed(); });

    // htmx:error: a request htmx 4 could not send; a cancelled one is no
    // error. htmx 2 dispatches it along with htmx:responseError and
    // htmx:sendError, handled above, and htmx 4 for errors of scripts,
    // which have no request.
    document.addEventListener("htmx:error", function (event) {
        var detail = event.detail || {};
        if (detail.errorInfo || !detail.ctx) return;
        if (detail.error && detail.error.name === "AbortError") return;
        // A response with an error status counts at htmx:finally:request.
        if (failedResponses.has(detail.ctx)) return;
        requestFailed();
    });

    [
        "up:request:offline", "up:fragment:offline",
        "turbo:fetch-request-error",
        "partial:error"
    ].forEach(function (name) {
        document.addEventListener(name, function () { requestFailed(); });
    });

    // A new request removes the request errors of earlier ones, with
    // data-flash-request-error-replace.
    [
        "htmx:before:request", "htmx:beforeRequest",
        "up:request:load", "turbo:before-fetch-request"
    ].forEach(function (name) {
        document.addEventListener(name, function () {
            if (options().replace) dismissRequestErrors();
        });
    });

    document.addEventListener("click", function (event) {
        var button = event.target.closest && event.target.closest("[data-flash-dismiss]");
        if (!button) return;
        var message = button.closest(selector);
        if (message) dismiss(message);
    });

    // Escape dismisses the message that has the focus.
    document.addEventListener("keydown", function (event) {
        if (event.key !== "Escape" || !event.target.closest) return;
        var message = event.target.closest(selector);
        if (message) dismiss(message);
    });

    function start() {
        scan(document.documentElement);
        new MutationObserver(function (records) {
            records.forEach(function (record) {
                Array.prototype.forEach.call(record.addedNodes, function (node) {
                    if (node.nodeType === 1) scan(node);
                });
            });
        }).observe(document.documentElement, { childList: true, subtree: true });
    }

    global.goPartialFlash = {
        show: function (level, text) { return show(level, text, false); },
        dismiss: dismiss,
        scan: scan
    };

    if (document.readyState === "loading") {
        document.addEventListener("DOMContentLoaded", start);
    } else {
        start();
    }
})(window);
