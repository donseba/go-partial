(function() {
    htmx.on('htmx:configRequest', function(event) {
        let element = event.detail.elt;
        let selectValue = element.getAttribute('x-select');
        if (selectValue !== null) {
            event.detail.headers['X-Select'] = selectValue;
        }

        let actionValue = element.getAttribute('x-action');
        if (actionValue !== null) {
            event.detail.headers['X-Action'] = actionValue;
        }
    });
})();