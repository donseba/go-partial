// go-partial SSE client: applies the partial:patch events of exp/sse to the
// page and passes partial:signal and partial:error on as DOM events.
//
// Elements with data-partial-stream="/url" (or the data-partial-interaction
// ="stream" markup of {{ stream }}) open a stream while they are in the page;
// script can open one with goPartialSSE.connect(url). A patch is
// {"target": "#id", "html": "..."}, as exp/sse writes it; its HTML replaces
// the elements the target selects (outerHTML). When htmx is loaded, patched
// content is processed by it.
//
// An element with data-partial-stream-hidden="30s" closes its stream when
// the tab has been hidden that long, and opens it again when the tab shows,
// dispatching partial:stream:reconnect on the element so the page can catch
// up. Changing the element's URL opens a stream to the new one.
//
// An element with data-partial-stream-client-id="tab" adds the client id
// of the tab to its URL as that parameter (?tab=...), so the server tells
// the tabs of one browser apart, such as for presence; the id stays the
// same for the life of the page, also across the swaps that replace the
// element, and scripts read it as goPartialSSE.clientId, to send with
// their own requests. A page restored from the back/forward cache opens
// its streams again, dispatching partial:stream:reconnect.
//
// An element with data-partial-stream-beacon="/url" tells that address
// about its stream, such as for presence, where the server must tell a tab
// that is hidden, and whose stream is closed, from one that is gone: it
// POSTs clientId (goPartialSSE.clientId), params (the query of the stream
// URL, with the client id parameter) and reason, as a form, with
// navigator.sendBeacon where the page may be going away:
//
//	beat      every data-partial-stream-beat="15s", while the element is in
//	          the page, also while the tab is hidden and its stream closed;
//	          an answer with a 4xx status stops them
//	hidden    the stream closed as the tab is hidden (data-partial-stream-hidden)
//	removed   the element left the page, and no element that is left
//	          reports to the same address, such as after a navigation to a
//	          page without the stream
//	pagehide  the page is being left, or kept in the back/forward cache
//
// exp/sse's ReadBeacon reads them on the server.
(function (global) {
    "use strict";

    if (!global.document || global.goPartialSSE) return;

    var document = global.document;
    var streams = new Map();
    var clientId = newClientId();

    function newClientId() {
        var crypto = global.crypto;
        if (crypto && typeof crypto.randomUUID === "function") return crypto.randomUUID();
        return Date.now().toString(36) + "-" + Math.random().toString(36).slice(2);
    }

    function emit(target, name, detail) {
        target.dispatchEvent(new CustomEvent(name, { bubbles: true, detail: detail }));
    }

    function parse(data) {
        try {
            return JSON.parse(data);
        } catch (error) {
            return data;
        }
    }

    // nodes parses the HTML of an event into nodes. Their style attributes
    // and inline <style> and <script> elements are subject to the page's
    // Content-Security-Policy, as if they had been in the page.
    function nodes(html) {
        var template = document.createElement("template");
        template.innerHTML = html;
        return Array.prototype.slice.call(template.content.childNodes);
    }

    // replace puts the nodes of html in the place of element, and returns
    // them.
    function replace(element, html) {
        var added = nodes(html);
        element.replaceWith.apply(element, added);
        return added;
    }

    function apply(patch) {
        if (!patch || !patch.target) return;
        var targets = document.querySelectorAll(patch.target);
        Array.prototype.forEach.call(targets, function (element) {
            var added = replace(element, patch.html || "");
            added.forEach(function (node) {
                if (node.nodeType !== 1) return;
                if (global.htmx && typeof global.htmx.process === "function") global.htmx.process(node);
                connectWithin(node);
            });
        });
        emit(document, "partial:patched", patch);
    }

    function connect(url, options) {
        var source = new EventSource(url, options);
        source.addEventListener("partial:patch", function (event) {
            apply(parse(event.data));
        });
        source.addEventListener("partial:signal", function (event) {
            var signal = parse(event.data);
            emit(document, "partial:signal", signal);
            if (signal && signal.name) emit(document, "partial:signal:" + signal.name, signal.value);
        });
        source.addEventListener("partial:error", function (event) {
            emit(document, "partial:error", parse(event.data));
        });
        return source;
    }

    // duration reads "30s", "500ms", "2m" or a number of milliseconds.
    function duration(value) {
        var match = /^\s*(\d+(?:\.\d+)?)\s*(ms|s|m)?\s*$/.exec(value || "");
        if (!match) return null;
        var amount = parseFloat(match[1]);
        if (match[2] === "s") return amount * 1000;
        if (match[2] === "m") return amount * 60000;
        return amount;
    }

    function streamURL(element) {
        var url = null;
        if (element.hasAttribute("data-partial-stream")) url = element.getAttribute("data-partial-stream");
        else if (element.getAttribute("data-partial-interaction") === "stream") url = element.getAttribute("data-url");
        if (!url) return url;
        return withClientId(element, url);
    }

    // withClientId adds the tab's client id to url as the parameter that
    // data-partial-stream-client-id names ("client" when it is empty).
    function withClientId(element, url) {
        if (!element.hasAttribute("data-partial-stream-client-id")) return url;
        var name = element.getAttribute("data-partial-stream-client-id") || "client";
        try {
            var parsed = new URL(url, document.baseURI);
            parsed.searchParams.set(name, clientId);
            if (parsed.origin === global.location.origin) return parsed.pathname + parsed.search + parsed.hash;
            return parsed.toString();
        } catch (error) {
            return url + (url.indexOf("?") >= 0 ? "&" : "?") + encodeURIComponent(name) + "=" + encodeURIComponent(clientId);
        }
    }

    var selector = "[data-partial-stream],[data-partial-interaction=stream]";

    // open starts the stream of an element, and its beats; streams maps
    // elements to {url, source, timer, beat}, source being null while the
    // tab is hidden.
    function open(element, url) {
        var stream = streams.get(element);
        if (!stream) {
            stream = { url: url, source: null, timer: null, beat: null };
            streams.set(element, stream);
        }

        stream.url = url;
        stream.source = connect(url, { withCredentials: element.hasAttribute("data-partial-credentials") });
        startBeat(element, stream);
    }

    // close ends the stream of an element, and tells its beacon the reason,
    // if any.
    function close(element, reason) {
        var stream = streams.get(element);
        if (!stream) return;
        if (stream.source) stream.source.close();
        if (stream.timer) clearTimeout(stream.timer);
        stopBeat(stream);
        streams.delete(element);
        if (reason) report(element, stream, reason);
    }

    // beacon returns the address an element reports its stream to, or "".
    function beacon(element) {
        var url = element.getAttribute("data-partial-stream-beacon") || "";
        if (!url) return "";
        try {
            return new URL(url, document.baseURI).toString();
        } catch (error) {
            return url;
        }
    }

    // query returns the query of a stream URL, without its "?".
    function query(url) {
        try {
            return new URL(url, document.baseURI).search.slice(1);
        } catch (error) {
            var index = url.indexOf("?");
            return index >= 0 ? url.slice(index + 1) : "";
        }
    }

    // report tells the beacon of an element about its stream: a beat with
    // fetch, which stops the beats on a 4xx answer, anything else with
    // navigator.sendBeacon, which the browser sends also when the page is
    // going away.
    function report(element, stream, reason) {
        var url = beacon(element);
        if (!url) return;

        var body = new URLSearchParams();
        body.set("clientId", clientId);
        body.set("reason", reason);
        body.set("params", query(stream.url));

        if (reason !== "beat" && global.navigator && typeof global.navigator.sendBeacon === "function") {
            if (global.navigator.sendBeacon(url, body)) return;
        }
        if (typeof global.fetch !== "function") return;

        var credentials = element.hasAttribute("data-partial-credentials") ? "include" : "same-origin";
        global.fetch(url, { method: "POST", body: body, credentials: credentials, keepalive: reason !== "beat" }).then(function (response) {
            if (reason === "beat" && response.status >= 400 && response.status < 500) {
                stream.beatStopped = true;
                stopBeat(stream);
            }
        }).catch(function () {
            // The next beat tries again.
        });
    }

    // startBeat reports the stream of an element every
    // data-partial-stream-beat, until it closes.
    function startBeat(element, stream) {
        if (stream.beat || stream.beatStopped || !beacon(element)) return;
        var interval = duration(element.getAttribute("data-partial-stream-beat"));
        if (!interval) return;
        stream.beat = setInterval(function () {
            report(element, stream, "beat");
        }, interval);
    }

    function stopBeat(stream) {
        if (!stream.beat) return;
        clearInterval(stream.beat);
        stream.beat = null;
    }

    function connectWithin(root) {
        var elements = root.matches && root.matches(selector) ? [root] : [];
        if (root.querySelectorAll) elements = elements.concat(Array.prototype.slice.call(root.querySelectorAll(selector)));
        elements.forEach(function (element) {
            var url = streamURL(element);
            if (!url || streams.has(element)) return;
            open(element, url);
        });
    }

    // An element whose URL changes streams from the new one. It stays in
    // the page, so its beacon hears of no removal.
    function changed(element) {
        var stream = streams.get(element);
        var url = streamURL(element);
        if (stream && stream.url === url) return;
        close(element);
        if (url && element.isConnected) open(element, url);
    }

    // reported reports whether an element in the page reports its stream
    // to url.
    function reported(url) {
        var found = false;
        streams.forEach(function (stream, element) {
            if (element.isConnected && beacon(element) === url) found = true;
        });
        return found;
    }

    // Streams end with their element, such as when a swap replaces it. Its
    // beacon hears of it, unless an element in the page reports to it,
    // such as the one that replaced it.
    function closeRemoved() {
        var removed = [];
        streams.forEach(function (stream, element) {
            if (!element.isConnected) removed.push(element);
        });
        removed.forEach(function (element) {
            var url = beacon(element);
            close(element, url && !reported(url) ? "removed" : "");
        });
    }

    // While the tab is hidden, streams of elements with
    // data-partial-stream-hidden close after that grace period; they open
    // again when it shows.
    function visibility() {
        streams.forEach(function (stream, element) {
            var grace = duration(element.getAttribute("data-partial-stream-hidden"));
            if (grace === null) return;

            if (document.hidden) {
                if (stream.timer || !stream.source) return;
                stream.timer = setTimeout(function () {
                    stream.timer = null;
                    if (stream.source) stream.source.close();
                    stream.source = null;
                    emit(element, "partial:stream:paused", { url: stream.url });
                    report(element, stream, "hidden");
                }, grace);
                return;
            }

            if (stream.timer) {
                clearTimeout(stream.timer);
                stream.timer = null;
            }
            if (stream.source) return;
            open(element, stream.url);
            emit(element, "partial:stream:reconnect", { url: stream.url });
        });
    }

    function start() {
        connectWithin(document.documentElement);
        new MutationObserver(function (records) {
            records.forEach(function (record) {
                if (record.type === "attributes") {
                    if (record.target.nodeType === 1) changed(record.target);
                    return;
                }
                Array.prototype.forEach.call(record.addedNodes, function (node) {
                    if (node.nodeType === 1) connectWithin(node);
                });
            });
            closeRemoved();
        }).observe(document.documentElement, {
            childList: true,
            subtree: true,
            attributes: true,
            attributeFilter: ["data-partial-stream", "data-partial-interaction", "data-url", "data-partial-stream-client-id"]
        });
        document.addEventListener("visibilitychange", visibility);

        // A page that is left tells the beacons of its streams. One kept in
        // the back/forward cache closes its streams, so the browser can
        // keep it, and opens them again when it is restored.
        global.addEventListener("pagehide", function (event) {
            streams.forEach(function (stream, element) {
                report(element, stream, "pagehide");
            });
            if (!event.persisted) return;
            streams.forEach(function (stream) {
                if (stream.timer) clearTimeout(stream.timer);
                stream.timer = null;
                stopBeat(stream);
                if (stream.source) stream.source.close();
                stream.source = null;
            });
        });
        global.addEventListener("pageshow", function (event) {
            if (!event.persisted) return;
            streams.forEach(function (stream, element) {
                if (stream.source && stream.source.readyState !== 2) return;
                if (stream.source) stream.source.close();
                open(element, stream.url);
                emit(element, "partial:stream:reconnect", { url: stream.url });
            });
            connectWithin(document.documentElement);
        });
    }

    global.goPartialSSE = { connect: connect, apply: apply, clientId: clientId };

    if (document.readyState === "loading") {
        document.addEventListener("DOMContentLoaded", start);
    } else {
        start();
    }
})(window);
