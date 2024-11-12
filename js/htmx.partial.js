(function() {
    htmx.on('htmx:configRequest', function(event) {
        var element = event.detail.elt;
        var partialValue = element.getAttribute('hx-partial');
        if (partialValue !== null) {
            event.detail.headers['X-Partial'] = partialValue;
        }
    });
})();