// @ts-check

/**
 * @typedef {Object} PartialOptions
 * @property {'outerHTML'|'innerHTML'} [defaultSwapOption='outerHTML'] - Default swap method.
 * @property {Function} [onError] - Callback function for handling errors.
 * @property {Function|string} [csrfToken] - CSRF token value or function returning the token.
 * @property {Function} [beforeRequest] - Hook before the request is sent.
 * @property {Function} [afterResponse] - Hook after the response is received.
 * @property {boolean} [autoFocus=true] - Whether to auto-focus the target element after content update.
 * @property {number} [debounceTime=0] - Debounce time in milliseconds for event handlers.
 */

/**
 * Class representing Partial.
 */
class Partial {
    /**
     * Creates an instance of Partial.
     * @param {PartialOptions} [options={}] - Configuration options.
     */
    constructor(options = {}) {
        // Define the custom action attributes
        this.actionAttributes = ['x-get', 'x-post', 'x-put', 'x-delete'];

        // Store options with default values
        this.onError = options.onError || null;
        this.csrfToken = options.csrfToken || null;
        this.defaultSwapOption = options.defaultSwapOption || 'outerHTML';
        this.beforeRequest = options.beforeRequest || null;
        this.afterResponse = options.afterResponse || null;
        this.autoFocus = options.autoFocus !== undefined ? options.autoFocus : false;
        this.debounceTime = options.debounceTime || 0;

        this.eventTarget = new EventTarget();
        this.eventListeners = {};

        // Bind methods to ensure correct 'this' context
        this.scanForElements = this.scanForElements.bind(this);
        this.setupElement = this.setupElement.bind(this);
        this.handleAction = this.handleAction.bind(this);
        this.handleOobSwapping = this.handleOobSwapping.bind(this);
        this.handlePopState = this.handlePopState.bind(this);

        // Initialize the handler on DOMContentLoaded
        document.addEventListener('DOMContentLoaded', () => this.scanForElements());

        // Listen for popstate events
        window.addEventListener('popstate', this.handlePopState);
    }

    /**
     * Scans the entire document or a specific container for elements with defined action attributes.
     * @param {HTMLElement | Document} [container=document]
     */
    scanForElements(container = document) {
        const selector = this.actionAttributes.map(attr => `[${attr}]`).join(',');
        const elements = container.querySelectorAll(selector);
        elements.forEach(this.setupElement);
    }

    /**
     * Sets up an individual element by attaching the appropriate event listener.
     * @param {HTMLElement} element
     */
    setupElement(element) {
        // Avoid attaching multiple listeners
        if (element.__xRequestHandlerInitialized) return;

        // Set a default trigger based on the element type
        let trigger;
        if (element.tagName === 'FORM') {
            trigger = element.getAttribute('x-trigger') || 'submit';
        } else {
            trigger = element.getAttribute('x-trigger') || 'click';
        }

        // Get custom debounce time from x-debounce attribute
        let elementDebounceTime = this.debounceTime; // Default to global debounce time
        const xDebounce = element.getAttribute('x-debounce');
        if (xDebounce !== null) {
            const parsedDebounce = parseInt(xDebounce, 10);
            if (!isNaN(parsedDebounce) && parsedDebounce >= 0) {
                elementDebounceTime = parsedDebounce;
            } else {
                console.warn(`Invalid x-debounce value '${xDebounce}' on element:`, element);
            }
        }

        // Debounce only the handleAction function
        const debouncedHandleAction = this.debounce((event) => {
            this.handleAction(event, element);
        }, elementDebounceTime);

        // Event handler that calls preventDefault immediately
        const handler = (event) => {
            event.preventDefault();
            debouncedHandleAction(event);
        };

        element.addEventListener(trigger, handler);

        // Mark the element as initialized
        element.__xRequestHandlerInitialized = true;
    }

    /**
     * Handles the action when an element is triggered.
     * @param {Event} event
     * @param {HTMLElement} element
     */
    async handleAction(event, element) {
        const method = this.getMethod(element);
        let url = element.getAttribute(`x-${method.toLowerCase()}`);

        if (!url) {
            const error = new Error(`No URL specified for method ${method} on element.`);
            console.error(error.message, element);
            if (typeof this.onError === 'function') {
                this.onError(error, element);
            }
            return;
        }

        const headers = this.getHeaders(element);
        let partialSelector = element.getAttribute('x-target');
        if (!partialSelector) {
            partialSelector = window.body;
        }

        const targetElement = document.querySelector(partialSelector);
        if (!targetElement) {
            const error = new Error(`No element found with selector '${partialSelector}' for 'x-target' targeting.`);
            console.error(error.message);
            if (typeof this.onError === 'function') {
                this.onError(error, element);
            }
            return;
        }

        const partialId = targetElement.getAttribute('id');
        if (!partialId) {
            const error = new Error(`Target element does not have an 'id' attribute.`);
            console.error(error.message, targetElement);
            if (typeof this.onError === 'function') {
                this.onError(error, element);
            }
            return;
        }

        // Set the X-Target header to the request
        headers["X-Target"] = partialId;

        // Handle x-params for GET requests
        if (method === 'GET') {
            const xParams = element.getAttribute('x-params');
            if (xParams) {
                try {
                    const paramsObject = JSON.parse(xParams);
                    const urlParams = new URLSearchParams(paramsObject).toString();
                    url += (url.includes('?') ? '&' : '?') + urlParams;
                } catch (e) {
                    console.error('Invalid JSON in x-params attribute:', e);
                    const error = new Error('Invalid JSON in x-params attribute');
                    if (typeof this.onError === 'function') {
                        this.onError(error, element);
                    }
                    return;
                }
            }
        }

        try {
            // Dispatch x-before event(s) if specified
            const beforeEvents = element.getAttribute('x-before');
            if (beforeEvents) {
                await this.dispatchCustomEvents(beforeEvents, { element, event });
            }

            // Before request hook
            if (typeof this.beforeRequest === 'function') {
                await this.beforeRequest({ method, url, headers, element });
            }

            const responseText = await this.performRequest(method, url, headers, element);

            // After response hook
            if (typeof this.afterResponse === 'function') {
                await this.afterResponse({ response: this.lastResponse, element });
            }

            // Parse the response HTML
            const parser = new DOMParser();
            const doc = parser.parseFromString(responseText, 'text/html');

            // Extract OOB elements
            const oobElements = Array.from(doc.querySelectorAll('[x-swap-oob]'));
            oobElements.forEach(el => el.parentNode.removeChild(el));

            // Dispatch an event before the content is replaced
            this.dispatchEvent('xBeforeContentReplace', { targetElement, doc });

            // Replace the target's innerHTML with the main content
            targetElement.innerHTML = doc.body.innerHTML;

            // Dispatch an event after the content is replaced
            this.dispatchEvent('xAfterContentReplace', { targetElement, doc });

            // After successfully updating content
            const shouldPushState = element.getAttribute('x-push-state') !== 'false';
            if (shouldPushState) {
                const newUrl = new URL(url, window.location.origin);
                history.pushState({ xPartial: true }, '', newUrl);
            }

            // Auto-focus if enabled
            const focusEnabled = element.getAttribute('x-focus') !== 'false';
            if (this.autoFocus && focusEnabled) {
                if (targetElement.getAttribute('tabindex') === null) {
                    targetElement.setAttribute('tabindex', '-1');
                }
                targetElement.focus();
            }

            // Re-scan the newly added content for Partial elements
            this.scanForElements(targetElement);

            // Handle OOB swapping with the extracted OOB elements
            this.handleOobSwapping(oobElements);

            // Handle any x-event-* headers from the response
            await this.handleResponseEvents();

            // Dispatch x-after event(s) if specified
            const afterEvents = element.getAttribute('x-after');
            if (afterEvents) {
                await this.dispatchCustomEvents(afterEvents, { element, event });
            }

        } catch (error) {
            console.error('Request failed:', error);

            if (typeof this.onError === 'function') {
                this.onError(error, element);
            } else {
                // Optionally, handle error display in the UI
                targetElement.innerHTML = `<div class="error">An error occurred: ${error.message}</div>`;
            }
        }
    }

    /**
     * Dispatches custom events specified in a comma-separated string.
     * @param {string} events - Comma-separated event names.
     * @param {Object} detail - Detail object to pass with the event.
     */
    async dispatchCustomEvents(events, detail) {
        const eventNames = events.split(',').map(e => e.trim());
        for (const eventName of eventNames) {
            const event = new CustomEvent(eventName, { detail });
            this.eventTarget.dispatchEvent(event);
        }
    }

    /**
     * Handles the popstate event for browser navigation.
     * @param {PopStateEvent} event
     */
    async handlePopState(event) {
        if (event.state && event.state.xPartial) {
            const url = window.location.href;
            try {
                const responseText = await this.performRequest('GET', url, {}, null);

                // Parse the response HTML
                const parser = new DOMParser();
                const doc = parser.parseFromString(responseText, 'text/html');

                // Replace the body content
                document.body.innerHTML = doc.body.innerHTML;

                // Re-scan the entire document
                this.scanForElements();

                // Optionally, focus the body
                if (this.autoFocus) {
                    document.body.focus();
                }

            } catch (error) {
                console.error('PopState request failed:', error);
                if (typeof this.onError === 'function') {
                    this.onError(error, document.body);
                }
            }
        }
    }

    /**
     * Debounce function to limit the rate at which a function can fire.
     * @param {Function} func - The function to debounce.
     * @param {number} wait - The number of milliseconds to wait.
     * @returns {Function}
     */
    debounce(func, wait) {
        let timeout;
        return (...args) => {
            const later = () => {
                clearTimeout(timeout);
                func.apply(this, args);
            };
            clearTimeout(timeout);
            timeout = setTimeout(later, wait);
        };
    }

    /**
     * Determines the HTTP method based on the element's attributes.
     * @param {HTMLElement} element
     * @returns {string} HTTP method
     */
    getMethod(element) {
        for (const attr of this.actionAttributes) {
            if (element.hasAttribute(attr)) {
                return attr.replace('x-', '').toUpperCase();
            }
        }
        return 'GET'; // Default method
    }

    /**
     * Constructs headers from the element's attributes.
     * @param {HTMLElement} element
     * @returns {Object} Headers object
     */
    getHeaders(element) {
        const headers = {};

        if (this.csrfToken) {
            if (typeof this.csrfToken === 'function') {
                headers['X-CSRF-Token'] = this.csrfToken();
            } else {
                headers['X-CSRF-Token'] = this.csrfToken;
            }
        }

        // Collect all x-* attributes that are not actionAttributes
        for (const attr of element.attributes) {
            const name = attr.name;
            if (name.startsWith('x-') && !this.actionAttributes.includes(name)) {
                const headerName = 'X-' + this.capitalize(name.substring(2)); // Remove 'x-' prefix and capitalize
                headers[headerName] = attr.value;
            }
        }

        return headers;
    }

    /**
     * Capitalizes the first letter of the string.
     * @param {string} str
     * @returns {string}
     */
    capitalize(str) {
        return str.charAt(0).toUpperCase() + str.slice(1);
    }

    /**
     * Performs the HTTP request using Fetch API.
     * @param {string} method
     * @param {string} url
     * @param {Object} headers
     * @param {HTMLElement|null} element
     * @returns {Promise<string>} Response text
     */
    performRequest(method, url, headers, element) {
        const options = {
            method,
            headers,
            credentials: 'same-origin',
        };

        // Handle request body
        if (['POST', 'PUT', 'PATCH', 'DELETE'].includes(method)) {
            let body = null;

            // Check if the element or the closest form has x-serialize="json"
            const serializeAsJson = element && (element.getAttribute('x-serialize') === 'json' ||
                (element.closest('form') && element.closest('form').getAttribute('x-serialize') === 'json'));

            // Check for x-json attribute
            const xJson = element && element.getAttribute('x-json');

            if (xJson) {
                // Parse x-json attribute
                try {
                    body = JSON.stringify(JSON.parse(xJson));
                    headers['Content-Type'] = 'application/json';
                } catch (e) {
                    console.error('Invalid JSON in x-json attribute:', e);
                    throw new Error('Invalid JSON in x-json attribute');
                }
            } else if (element && (element.tagName === 'FORM' || element.closest('form'))) {
                const form = element.tagName === 'FORM' ? element : element.closest('form');
                if (serializeAsJson) {
                    // Serialize form data as JSON
                    const formData = new FormData(form);
                    const jsonObject = {};
                    formData.forEach((value, key) => {
                        // Handle multiple values per key (e.g., checkboxes)
                        if (jsonObject[key]) {
                            if (Array.isArray(jsonObject[key])) {
                                jsonObject[key].push(value);
                            } else {
                                jsonObject[key] = [jsonObject[key], value];
                            }
                        } else {
                            jsonObject[key] = value;
                        }
                    });
                    body = JSON.stringify(jsonObject);
                    headers['Content-Type'] = 'application/json';
                } else {
                    // Use FormData
                    body = new FormData(form);
                }
            }

            if (body) {
                options.body = body;
            }
        }

        return fetch(url, options).then(response => {
            // Store the response for event handling
            this.lastResponse = response;

            if (!response.ok) {
                return response.text().then(text => {
                    throw new Error(`HTTP error ${response.status}: ${text}`);
                });
            }
            return response.text();
        });
    }

    /**
     * Handles Out-of-Band (OOB) swapping by processing an array of OOB elements.
     * Replaces existing elements in the document with the new content based on matching IDs.
     * @param {HTMLElement[]} oobElements
     */
    handleOobSwapping(oobElements) {
        oobElements.forEach(oobElement => {
            const targetId = oobElement.getAttribute('id');
            if (!targetId) {
                console.error('OOB element does not have an ID:', oobElement);
                return;
            }

            const swapOption = oobElement.getAttribute('x-swap-oob') || this.defaultSwapOption;
            const existingElement = document.getElementById(targetId);

            if (!existingElement) {
                console.error(`No existing element found with ID '${targetId}' for OOB swapping.`);
                return;
            }

            if (swapOption === 'outerHTML' || swapOption === true) {
                existingElement.outerHTML = oobElement.outerHTML;
            } else if (swapOption === 'innerHTML') {
                existingElement.innerHTML = oobElement.innerHTML;
            } else {
                console.error(`Invalid x-swap-oob option '${swapOption}' on element with ID '${targetId}'. Use 'outerHTML' or 'innerHTML'.`);
                return;
            }

            // After swapping, initialize any new elements within the replaced content
            const newElement = document.getElementById(targetId);
            if (newElement) {
                this.scanForElements(newElement);
            }
        });
    }

    /**
     * Handles any x-event-* headers from the response and dispatches events accordingly.
     */
    async handleResponseEvents() {
        if (!this.lastResponse || !this.lastResponse.headers) {
            return;
        }

        this.lastResponse.headers.forEach((value, name) => {
            if (name.toLowerCase().startsWith('x-event-')) {
                const eventName = name.substring(8); // Remove 'x-event-' prefix
                let eventData = value;
                try {
                    eventData = JSON.parse(value);
                } catch (e) {
                    // Value is not JSON, use as is
                }
                this.dispatchEvent(eventName, eventData);
            }
        });
    }

    /**
     * Listens for a custom event and executes the callback when the event is dispatched.
     * @param {string} eventName - The name of the event to listen for
     * @param {Function} callback - The function to call when the event is dispatched
     * @param {boolean | AddEventListenerOptions} [options] - Optional options for addEventListener.
     */
    event(eventName, callback, options) {
        if (!this.eventListeners[eventName]) {
            this.eventListeners[eventName] = [];
        }
        this.eventListeners[eventName].push({ callback, options });
        this.eventTarget.addEventListener(eventName, callback, options);
    }

    /**
     * Removes a custom event listener.
     * @param {string} eventName - The name of the event to remove
     * @param {Function} callback - The function to remove
     * @param {boolean | AddEventListenerOptions} [options] - Optional options for addEventListener.
     */
    removeEvent(eventName, callback, options) {
        if (this.eventListeners[eventName]) {
            // Find the index of the listener to remove
            const index = this.eventListeners[eventName].findIndex(
                (listener) => listener.callback === callback && JSON.stringify(listener.options) === JSON.stringify(options)
            );
            if (index !== -1) {
                // Remove the listener from the registry
                this.eventListeners[eventName].splice(index, 1);
                // If no more listeners for this event, delete the event key
                if (this.eventListeners[eventName].length === 0) {
                    delete this.eventListeners[eventName];
                }
            }
        }

        this.eventTarget.removeEventListener(eventName, callback, options);
    }

    /**
     * Removes all event listeners for the given event name.
     * @param {string} eventName
     */
    removeAllEvents(eventName) {
        if (this.eventListeners[eventName]) {
            this.eventListeners[eventName].forEach(({ callback, options }) => {
                this.eventTarget.removeEventListener(eventName, callback, options);
            });
            delete this.eventListeners[eventName];
        }
    }

    /**
     * Dispatches a custom event with the given name and data.
     * @param {string} eventName
     * @param {any} eventData
     */
    dispatchEvent(eventName, eventData) {
        const event = new CustomEvent(eventName, { detail: eventData });
        this.eventTarget.dispatchEvent(event);
    }

    /**
     * Allows manually re-scanning a specific container for Partial elements.
     * Useful when dynamically adding content to the DOM.
     * @param {HTMLElement} container
     */
    refresh(container = document) {
        this.scanForElements(container);
    }
}
