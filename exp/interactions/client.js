// go-partial interactions client: the browser side of exp/interactions'
// attributes that a client library cannot do itself.
//
// A submit button with data-partial-confirm="Delete for good?" (clientAttrs
// "submitConfirm", connector.Attr.SubmitConfirm) asks that question before
// it sends its form, for a form whose buttons do different things; the
// form's own question, such as hx-confirm, is asked as well. Cancelling
// stops the submission, whichever library sends the form.
//
// A page with a <dialog data-partial-confirm-dialog> asks these questions,
// and htmx's hx-confirm (htmx 2 and 4), in that dialog instead of the
// browser's window.confirm, which some embedded browsers never show. The
// question goes into its element with data-partial-confirm-question, as
// text; a button in the dialog with a value closes it with that value, and
// "confirm" is the answer yes. Escape, or any other value, is no. The
// questions of Unpoly and Turbo, and an htmx 4 question that starts with
// js:, stay the library's own.
//
// A link or form in an element with data-partial-reload-on-error, or with
// it itself (clientAttrs "reloadOnError", connector.Attr.ReloadOnError),
// whose request gets a 5xx status or no answer at all loads as a whole page
// instead, so the visitor sees what went wrong rather than nothing: a link
// is followed and a GET form submitted again by the browser itself, with
// the button that sent it. A request that is not safe to send twice (POST,
// PUT, PATCH, DELETE) is never sent again: the HTML error page the server
// answered replaces the page, as a whole-page load would show it; without
// one, or without an answer, the page stays as it is and
// partial:reload-on-error:failed is dispatched on the link or form, with
// the method and status (0 without an answer). It handles the requests of
// htmx 2 and 4, and those of Unpoly and Turbo that get no answer; both
// libraries show a 5xx answer themselves. A cancelled request is no error,
// and neither is an htmx 4 answer with HX-Reswap: none, such as the error
// flash message of go-partial's ext/errors, whose out-of-band parts htmx 4
// swaps.
(function (global) {
    "use strict";

    if (!global.document || global.goPartialInteractions) return;

    var document = global.document;

    // confirmed asks the question of a button with the browser's
    // window.confirm; it reports false when the user cancels.
    function confirmed(button) {
        var question = button.getAttribute("data-partial-confirm");
        return !question || global.confirm(question);
    }

    var dialogAttr = "data-partial-confirm-dialog";

    // dialog returns the page's confirmation dialog, or null when the page
    // has none or the browser cannot show one.
    function dialog() {
        var found = document.querySelector("dialog[" + dialogAttr + "]");
        return found && typeof found.showModal === "function" ? found : null;
    }

    // ask asks question in the page's confirmation dialog, or else with
    // window.confirm, and resolves to true when the user confirms. While the
    // dialog is open, another question is answered no.
    function ask(question) {
        var found = dialog();
        if (!found) return Promise.resolve(global.confirm(question));
        if (found.open) return Promise.resolve(false);

        var text = found.querySelector("[data-partial-confirm-question]");
        if (text) text.textContent = question;

        return new Promise(function (resolve) {
            found.addEventListener("close", function () {
                resolve(found.returnValue === "confirm");
            }, { once: true });
            found.returnValue = "";
            found.showModal();
        });
    }

    // A button with a value closes the dialog with that value, also without
    // a <form method="dialog"> around it.
    document.addEventListener("click", function (event) {
        var button = event.target.closest && event.target.closest("dialog[" + dialogAttr + "] button[value]");
        if (!button) return;
        event.preventDefault();
        button.closest("dialog").close(button.value);
    });

    // pass lets the button through the checks below while send runs: the
    // click or submission it starts is the confirmed one.
    function pass(button, send) {
        button.setAttribute("data-partial-confirmed", "");
        setTimeout(function () { button.removeAttribute("data-partial-confirmed"); }, 0);
        if (send) send();
    }

    // The click is caught before any library sees it, in the capture phase;
    // a button that the keyboard or form.requestSubmit(button) uses is the
    // submit event's submitter. With a dialog, the click waits for the
    // answer and is made again after a yes.
    document.addEventListener("click", function (event) {
        var button = event.target.closest && event.target.closest("button[data-partial-confirm], input[data-partial-confirm]");
        if (!button || button.disabled || button.hasAttribute("data-partial-confirmed")) return;
        var question = button.getAttribute("data-partial-confirm");
        if (question && dialog()) {
            event.preventDefault();
            event.stopImmediatePropagation();
            ask(question).then(function (yes) {
                if (yes) pass(button, function () { button.click(); });
            });
            return;
        }
        if (confirmed(button)) {
            pass(button);
            return;
        }
        event.preventDefault();
        event.stopImmediatePropagation();
    }, true);

    document.addEventListener("submit", function (event) {
        var button = event.submitter;
        if (!button || !button.hasAttribute("data-partial-confirm")) return;
        if (button.hasAttribute("data-partial-confirmed")) return;
        var question = button.getAttribute("data-partial-confirm");
        var form = event.target;
        if (question && dialog()) {
            event.preventDefault();
            event.stopImmediatePropagation();
            ask(question).then(function (yes) {
                if (yes) pass(button, function () { form.requestSubmit(button); });
            });
            return;
        }
        if (confirmed(button)) return;
        event.preventDefault();
        event.stopImmediatePropagation();
    }, true);

    // htmx asks the question of hx-confirm in the page's dialog: htmx 4
    // waits for issueRequest or dropRequest, htmx 2 for issueRequest(true),
    // which skips its own question. Without a dialog, and for an htmx 4
    // question that starts with js:, htmx asks as it always does.
    document.addEventListener("htmx:confirm", function (event) {
        var detail = event.detail || {};
        var question = detail.ctx ? detail.ctx.confirm : detail.question;
        if (!question || !dialog() || /^(js|javascript):/.test(question)) return;
        event.preventDefault();
        ask(question).then(function (yes) {
            if (detail.ctx) {
                if (yes) detail.issueRequest();
                else detail.dropRequest();
                return;
            }
            if (yes) detail.issueRequest(true);
        });
    });

    var reloadAttr = "data-partial-reload-on-error";

    // reloadable returns the link or form of a failed request that loads
    // as a whole page, with the button that submitted the form: the
    // element that made the request, or the form of its submit button.
    function reloadable(element) {
        if (!element || element.nodeType !== 1 || !element.closest("[" + reloadAttr + "]")) return null;
        if (element.matches("a[href]")) return { element: element };
        if (element.matches("form")) return { element: element };

        var submit = element.matches("button, input[type=submit], input[type=image]") && element.form;
        if (submit && element.type !== "button" && element.type !== "reset") return { element: element.form, submitter: element };
        return null;
    }

    // method returns the method of a failed request, lowercase: the one
    // the library reports, else the form's, "get" for a link.
    function method(found, reported) {
        if (reported) return String(reported).toLowerCase();
        if (!found.element.matches("form")) return "get";
        var value = (found.submitter && found.submitter.getAttribute("formmethod")) || found.element.getAttribute("method") || "get";
        return value.toLowerCase();
    }

    // showPage replaces the page with an error page the server answered, as
    // a whole-page load would show it; it reports false without one. The
    // page written keeps the Content-Security-Policy of the page it
    // replaces, not the one of the error response: an inline <style> or
    // <script> of the error page runs only when this page's policy allows
    // it, such as go-partial's built-in error page with the hash of
    // ext/errors' StyleHash in style-src.
    function showPage(response) {
        if (!response || !/^text\/html/i.test(response.contentType || "")) return false;
        var html = response.text || "";
        if (!html.trim()) return false;
        document.open();
        document.write(html);
        document.close();
        return true;
    }

    // reload loads the link or form of a failed request as a whole page:
    // a link or a GET form is loaded again by the browser itself. A request
    // that is not safe to send twice, such as a POST of a comment, is never
    // sent again: its error page, if the server answered with one, replaces
    // the page; otherwise the page stays as it is and
    // partial:reload-on-error:failed is dispatched on the element.
    function reload(source, failure) {
        var found = reloadable(source);
        if (!found) return false;
        failure = failure || {};

        var element = found.element;
        var verb = method(found, failure.method);
        if (verb === "get" || verb === "head") {
            if (element.matches("a[href]")) {
                global.location.assign(element.href);
                return true;
            }
            resubmit(found);
            return true;
        }

        if (showPage(failure.response)) return true;

        element.dispatchEvent(new CustomEvent("partial:reload-on-error:failed", {
            bubbles: true,
            detail: { method: verb, status: failure.response ? failure.response.status : 0 }
        }));
        return false;
    }

    // resubmit sends a GET form again with the browser itself, with the
    // button that submitted it.
    function resubmit(found) {
        var element = found.element;
        var submitter = found.submitter;
        if (submitter && submitter.name) {
            var input = document.createElement("input");
            input.type = "hidden";
            input.name = submitter.name;
            input.value = submitter.value;
            element.appendChild(input);
        }
        // The prototype's submit sends the form without a submit event, so
        // no library takes it over again; a field named "submit" does not
        // hide it.
        HTMLFormElement.prototype.submit.call(element);
    }

    function aborted(error) {
        return !!error && error.name === "AbortError";
    }

    // htmx 4: a response with an error status, and a request that got no
    // answer.
    document.addEventListener("htmx:response:error", function (event) {
        var ctx = event.detail && event.detail.ctx;
        if (!ctx || !ctx.response || ctx.response.status < 500) return;
        var headers = ctx.response.headers;
        // HX-Reswap: none leaves the page as it is: the answer has only
        // out-of-band parts, such as an error flash message, which htmx 4
        // swaps.
        if (headers && headers.get && String(headers.get("HX-Reswap") || "").trim() === "none") return;
        reload(ctx.sourceElement || event.target, {
            method: ctx.request && ctx.request.method,
            response: {
                status: ctx.response.status,
                text: ctx.text,
                contentType: headers && headers.get ? headers.get("Content-Type") : ""
            }
        });
    });
    document.addEventListener("htmx:error", function (event) {
        var detail = event.detail || {};
        if (!detail.ctx || detail.errorInfo || aborted(detail.error)) return;
        if (detail.ctx.response) return;
        reload(detail.ctx.sourceElement || event.target, { method: detail.ctx.request && detail.ctx.request.method });
    });

    // htmx 2.
    document.addEventListener("htmx:responseError", function (event) {
        var detail = event.detail || {};
        var xhr = detail.xhr;
        if (!xhr || xhr.status < 500) return;
        reload(detail.elt || event.target, {
            method: detail.requestConfig && detail.requestConfig.verb,
            response: { status: xhr.status, text: xhr.responseText, contentType: xhr.getResponseHeader("Content-Type") }
        });
    });
    document.addEventListener("htmx:sendError", function (event) {
        var detail = event.detail || {};
        reload(detail.elt || event.target, { method: detail.requestConfig && detail.requestConfig.verb });
    });

    // Unpoly and Turbo: a request that got no answer.
    document.addEventListener("up:fragment:offline", function (event) {
        var origin = (event.renderOptions && event.renderOptions.origin) || (event.request && event.request.origin);
        reload(origin, { method: event.request && event.request.method });
    });
    document.addEventListener("turbo:fetch-request-error", function (event) {
        var detail = event.detail || {};
        if (aborted(detail.error)) return;
        var request = detail.request;
        reload(event.target, { method: request && (request.method || (request.fetchOptions && request.fetchOptions.method)) });
    });

    global.goPartialInteractions = { confirmed: confirmed, ask: ask };
})(window);
