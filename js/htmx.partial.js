(function() {
    htmx.on('htmx:configRequest', function(event) {
        let element = event.detail.elt;
        let partialValue = element.getAttribute('hx-partial');
        if (partialValue !== null) {
            event.detail.headers['X-Partial'] = partialValue;
        }

        let selectValue = element.getAttribute('hx-select');
        if (selectValue !== null) {
            event.detail.headers['X-Select'] = selectValue;
        }

        let actionValue = element.getAttribute('hx-action');
        if (actionValue !== null) {
            event.detail.headers['X-Action'] = actionValue;
        }
    });
})();